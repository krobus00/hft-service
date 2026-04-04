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

type klineWSSubscriberConfig struct {
	ExchangeName    entity.ExchangeName
	WSURLEnvKey     string
	DefaultWSURL    string
	Resync          func(ctx context.Context, conn *websocket.Conn, fallback []entity.KlineSubscription) ([]entity.KlineSubscription, klineResyncState, error)
	LoadResyncState func(ctx context.Context) (klineResyncState, error)
	HandleMessage   func(ctx context.Context, message []byte) error
}

func subscribeKlineDataWithAutoResync(ctx context.Context, cfg klineWSSubscriberConfig, subscriptions []entity.KlineSubscription) error {
	if cfg.HandleMessage == nil {
		return fmt.Errorf("%s handle message function is required", cfg.ExchangeName)
	}

	wsURL := strings.TrimSpace(os.Getenv(cfg.WSURLEnvKey))
	if wsURL == "" {
		wsURL = cfg.DefaultWSURL
	}

	wsHost, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("invalid %s ws url: %w", cfg.ExchangeName, err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	attempt := 0
	activeSubscriptions := subscriptions
	lastResyncState := klineResyncState{}

	if cfg.LoadResyncState != nil {
		state, err := cfg.LoadResyncState(ctx)
		if err != nil {
			logrus.WithError(err).Warnf("%s initial resync state load failed", cfg.ExchangeName)
		} else {
			lastResyncState = state
		}
	}

	if cfg.Resync != nil {
		if resyncedSubscriptions, state, err := cfg.Resync(ctx, nil, activeSubscriptions); err != nil {
			logrus.WithError(err).Warnf("%s initial resync failed; starting with existing subscriptions", cfg.ExchangeName)
		} else {
			activeSubscriptions = resyncedSubscriptions
			lastResyncState = state
		}
	}

	var resyncTicker *time.Ticker
	var resyncTickerC <-chan time.Time
	if cfg.Resync != nil && wsResyncInterval > 0 {
		resyncTicker = time.NewTicker(wsResyncInterval)
		resyncTickerC = resyncTicker.C
		defer resyncTicker.Stop()
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		logrus.Infof("connecting to %s", wsHost.String())
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

		logrus.Infof("connected to %s", wsHost.String())

		attempt = 0
		conn.SetPongHandler(func(string) error {
			return nil
		})

		for _, v := range activeSubscriptions {
			if v.Exchange != string(cfg.ExchangeName) {
				continue
			}

			logrus.Infof("start subscription for symbol: %s, interval: %s", v.Symbol, v.Interval)

			if err := conn.WriteMessage(websocket.TextMessage, []byte(v.Payload)); err != nil {
				_ = conn.Close()
				wait := wsReconnectDelay(attempt, rng)
				attempt++
				logrus.WithFields(logrus.Fields{"retry_in": wait.String(), "attempt": attempt}).Warnf("%s ws subscribe failed: %v", cfg.ExchangeName, err)
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
					if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
						logrus.Error(err)
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
				return nil
			case <-resyncTickerC:
				if cfg.LoadResyncState != nil {
					currentState, err := cfg.LoadResyncState(ctx)
					if err != nil {
						logrus.WithError(err).Warnf("%s resync state load failed", cfg.ExchangeName)
						continue
					}

					if currentState.Equal(lastResyncState) {
						continue
					}
				}

				resyncedSubscriptions, state, err := cfg.Resync(ctx, conn, activeSubscriptions)
				if err != nil {
					logrus.WithError(err).Warnf("%s resync failed", cfg.ExchangeName)
					continue
				}

				activeSubscriptions = resyncedSubscriptions
				lastResyncState = state
				shouldResubscribe = true
			case message := <-messageCh:
				err := cfg.HandleMessage(ctx, message)
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
