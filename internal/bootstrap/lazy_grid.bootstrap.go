package bootstrap

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/service"
	"github.com/krobus00/hft-service/internal/service/ordermanager"
	"github.com/sirupsen/logrus"
)

func StartLazyGridStrategy() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		wsConn *websocket.Conn
		mu     sync.RWMutex
	)

	tokocryptoExchange := ordermanager.NewTokocryptoExchange(config.Env.Exchanges[string(ordermanager.ExchangeTokoCrypto)], map[string]string{
		"tkoidr": "TKO_IDR",
	})

	orderManager := ordermanager.NewOrderManagerService(tokocryptoExchange)
	strategy := service.NewLazyGridStrategy(service.DefaultLazyGridConfig(), orderManager)

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}

			tokoWs := url.URL{
				Scheme: "wss",
				Host:   "stream-cloud.tokocrypto.site",
				Path:   "/stream",
			}
			tokoSub := map[string]any{
				"method": "SUBSCRIBE",
				"params": []string{
					"tkoidr@kline_1m",
				},
				"id": 1,
			}
			conn, err := runWS(ctx, tokoWs, tokoSub, func(ctx context.Context, message []byte) error {
				klineData, err := tokocryptoExchange.HandleKlineData(ctx, message)
				if err != nil {
					return err
				}
				strategy.OnPrice(ctx, klineData)
				return nil
			})
			if err != nil {
				logrus.Error("ws exited:", err)
			}

			if conn != nil {
				mu.Lock()
				wsConn = conn
				mu.Unlock()
			}

			if ctx.Err() != nil {
				return
			}

			logrus.Info("reconnecting in 3 seconds...")
			time.Sleep(3 * time.Second)
		}
	}()

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"ws connection": func(ctx context.Context) error {
			cancel()

			mu.RLock()
			conn := wsConn
			mu.RUnlock()

			if conn == nil {
				return nil
			}

			if err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
				return err
			}

			return conn.Close()
		},
	})

	<-wait
}
