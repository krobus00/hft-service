package exchange

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/sirupsen/logrus"
)

const (
	wsWriteTimeout = 10 * time.Second
	wsPongWait     = wsPingInterval*2 + 10*time.Second
)

type klineWSSubscriberConfig struct {
	ExchangeName           entity.ExchangeName
	WSURLEnvKey            string
	DefaultWSURL           string
	ResolveWSURL           func(subscriptions []entity.KlineSubscription) string
	NormalizeSubscriptions func(subscriptions []entity.KlineSubscription) []entity.KlineSubscription
	ResyncSubscriptions    func(ctx context.Context, conn *websocket.Conn, fallback []entity.KlineSubscription) ([]entity.KlineSubscription, klineResyncState, error)
	LoadCurrentResyncState func(ctx context.Context) (klineResyncState, error)
	HandleKlineMessage     func(ctx context.Context, message []byte) error
}

func subscribeKlineDataWithAutoResync(ctx context.Context, cfg klineWSSubscriberConfig, subscriptions []entity.KlineSubscription) error {
	if cfg.HandleKlineMessage == nil {
		return fmt.Errorf("%s handle message function is required", cfg.ExchangeName)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	attempt := 0
	activeSubscriptions := subscriptions
	if cfg.NormalizeSubscriptions != nil {
		activeSubscriptions = cfg.NormalizeSubscriptions(activeSubscriptions)
	}
	logrus.WithFields(logrus.Fields{
		"exchange":                  cfg.ExchangeName,
		"input_subscription_count":  len(subscriptions),
		"active_subscription_count": len(activeSubscriptions),
		"resync_interval":           wsResyncInterval.String(),
		"websocket_url_env_key":     cfg.WSURLEnvKey,
		"subscriptions_in_url":      cfg.ResolveWSURL != nil,
	}).Info("kline websocket subscriber starting")

	lastResyncState := klineResyncState{}

	if cfg.LoadCurrentResyncState != nil {
		state, err := cfg.LoadCurrentResyncState(ctx)
		if err != nil {
			logrus.WithError(err).Warnf("%s initial resync state load failed", cfg.ExchangeName)
		} else {
			lastResyncState = state
		}
	}

	if cfg.ResyncSubscriptions != nil {
		if resyncedSubscriptions, state, err := cfg.ResyncSubscriptions(ctx, nil, activeSubscriptions); err != nil {
			logrus.WithError(err).Warnf("%s initial resync failed; starting with existing subscriptions", cfg.ExchangeName)
		} else {
			logrus.WithFields(logrus.Fields{
				"exchange":                      cfg.ExchangeName,
				"subscriptions_before_resync":   len(activeSubscriptions),
				"subscriptions_after_resync":    len(resyncedSubscriptions),
				"symbol_mapping_updated_at":     state.SymbolMappingUpdatedAt.UTC().Format(time.RFC3339Nano),
				"kline_subscription_updated_at": state.KlineSubscriptionUpdatedAt.UTC().Format(time.RFC3339Nano),
			}).Info("initial exchange resync completed")

			activeSubscriptions = resyncedSubscriptions
			if cfg.NormalizeSubscriptions != nil {
				activeSubscriptions = cfg.NormalizeSubscriptions(activeSubscriptions)
			}
			lastResyncState = state
		}
	}

	var resyncTicker *time.Ticker
	var resyncTickerC <-chan time.Time
	if cfg.ResyncSubscriptions != nil && wsResyncInterval > 0 {
		resyncTicker = time.NewTicker(wsResyncInterval)
		resyncTickerC = resyncTicker.C
		defer resyncTicker.Stop()
	}

	for {
		if err := ctx.Err(); err != nil {
			logrus.WithFields(logrus.Fields{
				"exchange": cfg.ExchangeName,
			}).Info("kline websocket subscriber stopped")
			return nil
		}

		wsURL := strings.TrimSpace(os.Getenv(cfg.WSURLEnvKey))
		if wsURL == "" {
			wsURL = cfg.DefaultWSURL
		}
		if cfg.ResolveWSURL != nil {
			if resolved := strings.TrimSpace(cfg.ResolveWSURL(activeSubscriptions)); resolved != "" {
				wsURL = resolved
			}
		}

		wsHost, err := url.Parse(wsURL)
		if err != nil {
			return fmt.Errorf("invalid %s ws url: %w", cfg.ExchangeName, err)
		}

		logrus.WithFields(logrus.Fields{
			"exchange":           cfg.ExchangeName,
			"websocket_url":      wsHost.String(),
			"subscription_count": len(activeSubscriptions),
		}).Info("connecting exchange websocket")
		dialer := *websocket.DefaultDialer
		dialer.HandshakeTimeout = wsDialTimeout

		dialCtx, cancelDial := context.WithTimeout(ctx, wsDialTimeout)
		conn, _, err := dialer.DialContext(dialCtx, wsHost.String(), nil)
		cancelDial()
		if err != nil {
			wait := wsReconnectDelay(attempt, rng)
			attempt++
			logrus.WithFields(logrus.Fields{"retry_in": wait.String(), "attempt": attempt}).Warnf("%s ws dial failed: %v", cfg.ExchangeName, err)
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil
			}
		}

		logrus.WithFields(logrus.Fields{
			"exchange":      cfg.ExchangeName,
			"websocket_url": wsHost.String(),
		}).Info("exchange websocket connected")

		attempt = 0
		if err := conn.SetReadDeadline(time.Now().Add(wsPongWait)); err != nil {
			_ = conn.Close()
			wait := wsReconnectDelay(attempt, rng)
			attempt++
			logrus.WithFields(logrus.Fields{"retry_in": wait.String(), "attempt": attempt}).Warnf("%s ws set read deadline failed: %v", cfg.ExchangeName, err)
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil
			}
		}
		conn.SetPongHandler(func(string) error {
			if err := conn.SetReadDeadline(time.Now().Add(wsPongWait)); err != nil {
				logrus.WithError(err).Warnf("%s ws set read deadline on pong failed", cfg.ExchangeName)
			}
			return nil
		})

		for _, v := range activeSubscriptions {
			if v.Exchange != string(cfg.ExchangeName) {
				continue
			}
			if strings.TrimSpace(v.Payload) == "" {
				continue
			}

			logrus.WithFields(logrus.Fields{
				"exchange": cfg.ExchangeName,
				"symbol":   v.Symbol,
				"interval": v.Interval,
			}).Info("sending kline websocket subscription")
			logrus.WithFields(logrus.Fields{
				"exchange": cfg.ExchangeName,
				"symbol":   v.Symbol,
				"interval": v.Interval,
				"payload":  v.Payload,
			}).Debug("kline websocket subscription payload")

			if err := conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
				_ = conn.Close()
				wait := wsReconnectDelay(attempt, rng)
				attempt++
				logrus.WithFields(logrus.Fields{"retry_in": wait.String(), "attempt": attempt, "symbol": v.Symbol, "interval": v.Interval}).Warnf("%s ws set write deadline failed: %v", cfg.ExchangeName, err)
				select {
				case <-time.After(wait):
					continue
				case <-ctx.Done():
					return nil
				}
			}

			if err := conn.WriteMessage(websocket.TextMessage, []byte(v.Payload)); err != nil {
				_ = conn.Close()
				wait := wsReconnectDelay(attempt, rng)
				attempt++
				logrus.WithFields(logrus.Fields{"retry_in": wait.String(), "attempt": attempt, "symbol": v.Symbol, "interval": v.Interval}).Warnf("%s ws subscribe failed: %v", cfg.ExchangeName, err)
				select {
				case <-time.After(wait):
					continue
				case <-ctx.Done():
					return nil
				}
			}
		}

		stopPing := make(chan struct{})
		go func(c *websocket.Conn) {
			ticker := time.NewTicker(wsPingInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if err := c.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
						logrus.WithError(err).Warnf("%s ws set write deadline for ping failed", cfg.ExchangeName)
						_ = c.Close()
						return
					}
					if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
						logrus.WithError(err).Warnf("%s ws ping failed; reconnecting", cfg.ExchangeName)
						_ = c.Close()
						return
					}
				case <-ctx.Done():
					return
				case <-stopPing:
					return
				}
			}
		}(conn)

		ctxDone := make(chan struct{})
		go func(c *websocket.Conn) {
			select {
			case <-ctx.Done():
				_ = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				_ = c.Close()
			case <-ctxDone:
			}
		}(conn)

		messageCh := make(chan []byte, 1)
		readErrCh := make(chan error, 1)
		go func(c *websocket.Conn) {
			for {
				_, message, err := c.ReadMessage()
				if err != nil {
					select {
					case readErrCh <- err:
					default:
					}
					return
				}

				select {
				case messageCh <- message:
				case <-ctx.Done():
					return
				case <-stopPing:
					return
				}
			}
		}(conn)

		readErr := false
		shouldResubscribe := false
	readLoop:
		for {
			select {
			case <-ctx.Done():
				close(stopPing)
				close(ctxDone)
				logrus.WithFields(logrus.Fields{
					"exchange": cfg.ExchangeName,
				}).Info("kline websocket subscriber stopped")
				return nil
			case <-resyncTickerC:
				if cfg.LoadCurrentResyncState != nil {
					currentState, err := cfg.LoadCurrentResyncState(ctx)
					if err != nil {
						logrus.WithError(err).Warnf("%s resync state load failed", cfg.ExchangeName)
						continue
					}

					if currentState.Equal(lastResyncState) {
						continue
					}

					logrus.WithFields(logrus.Fields{
						"exchange":                             cfg.ExchangeName,
						"symbol_mapping_updated_at_old":        lastResyncState.SymbolMappingUpdatedAt.UTC().Format(time.RFC3339Nano),
						"symbol_mapping_updated_at_latest":     currentState.SymbolMappingUpdatedAt.UTC().Format(time.RFC3339Nano),
						"kline_subscription_updated_at_old":    lastResyncState.KlineSubscriptionUpdatedAt.UTC().Format(time.RFC3339Nano),
						"kline_subscription_updated_at_latest": currentState.KlineSubscriptionUpdatedAt.UTC().Format(time.RFC3339Nano),
						"active_subscriptions":                 len(activeSubscriptions),
					}).Info("detected exchange resync changes for subscriptions or symbol mappings")
				}

				previousCount := len(activeSubscriptions)
				resyncedSubscriptions, state, err := cfg.ResyncSubscriptions(ctx, conn, activeSubscriptions)
				if err != nil {
					logrus.WithError(err).Warnf("%s resync failed", cfg.ExchangeName)
					continue
				}

				logrus.WithFields(logrus.Fields{
					"exchange":                      cfg.ExchangeName,
					"subscriptions_before_resync":   previousCount,
					"subscriptions_after_resync":    len(resyncedSubscriptions),
					"symbol_mapping_updated_at":     state.SymbolMappingUpdatedAt.UTC().Format(time.RFC3339Nano),
					"kline_subscription_updated_at": state.KlineSubscriptionUpdatedAt.UTC().Format(time.RFC3339Nano),
				}).Info("exchange resync completed")

				activeSubscriptions = resyncedSubscriptions
				if cfg.NormalizeSubscriptions != nil {
					activeSubscriptions = cfg.NormalizeSubscriptions(activeSubscriptions)
				}
				lastResyncState = state
				shouldResubscribe = true
			case message := <-messageCh:
				err := cfg.HandleKlineMessage(ctx, message)
				if err != nil {
					logrus.Errorf("%s ws handle kline data failed: %v", cfg.ExchangeName, err)
					continue
				}
			case err := <-readErrCh:
				if ctx.Err() != nil {
					close(stopPing)
					close(ctxDone)
					return nil
				}

				if shouldResubscribe {
					break readLoop
				}

				readErr = true
				logrus.Errorf("%s ws read failed: %v", cfg.ExchangeName, err)
				break readLoop
			}
		}

		close(stopPing)
		close(ctxDone)
		_ = conn.Close()

		if shouldResubscribe {
			logrus.Infof("%s websocket resync completed; reconnecting with latest subscriptions", cfg.ExchangeName)
			continue
		}

		if !readErr {
			continue
		}

		wait := wsReconnectDelay(attempt, rng)
		attempt++
		logrus.WithFields(logrus.Fields{"retry_in": wait.String(), "attempt": attempt}).Warnf("reconnecting %s ws", cfg.ExchangeName)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil
		}
	}
}
