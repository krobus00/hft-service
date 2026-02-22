package bootstrap

import (
	"context"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/service"
	"github.com/sirupsen/logrus"
)

func StartLazyGridStrategy() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		wsConn *websocket.Conn
		mu     sync.RWMutex
	)

	// tokocryptoExchange := exchange.InitTokocryptoExchange(ctx, config.Env.Exchanges[string(entity.ExchangeTokoCrypto)], nil)

	// orderManager := ordermanager.NewOrderManagerService(tokocryptoExchange)
	stateStore, err := service.NewRedisLazyGridStateStore(config.Env.Redis.MarketData.CacheDSN)
	if err != nil {
		logrus.Fatal(err)
	}

	// strategy, err := service.NewLazyGridStrategy(ctx, service.DefaultLazyGridConfig(), orderManager, stateStore)
	// if err != nil {
	// 	logrus.Fatal(err)
	// }

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"redis connection": func(ctx context.Context) error {
			return stateStore.Close()
		},
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
