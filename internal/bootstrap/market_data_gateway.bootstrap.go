package bootstrap

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/sirupsen/logrus"
)

func StartMarketDataGateway() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		wsConn *websocket.Conn
		mu     sync.RWMutex
	)

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
					"solidr@kline_1m",
				},
				"id": 1,
			}
			conn, err := runWS(ctx, tokoWs, tokoSub, func(ctx context.Context, message []byte) error {
				logrus.Infof("received: %s", message)
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
