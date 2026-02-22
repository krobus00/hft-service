package bootstrap

import (
	"context"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/service/exchange"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func StartMarketDataWorker(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["market_data"])
	util.ContinueOrFatal(err)
	infrastructure.StartPostgresHealthCheck(ctx, db, config.Env.Database["market_data"].PingInterval)

	nc, js, err := infrastructure.NewJetstream()
	util.ContinueOrFatal(err)

	symbolMappingRepo := repository.NewSymbolMappingRepository(db)
	marketKlineRepo := repository.NewMarketKlineRepository(db)

	exchange.InitTokocryptoExchange(ctx, config.Env.Exchanges[string(entity.ExchangeTokoCrypto)], symbolMappingRepo, js, marketKlineRepo)

	publishers := make([]entity.Publisher, 0)
	for key, v := range exchange.GlobalExchangeRegistry {
		if publisher, ok := v.(entity.Publisher); ok {
			publishers = append(publishers, publisher)
			logrus.Info("added publisher for exchange: ", key)
		}
	}

	subscribers := make([]entity.Subscriber, 0)
	for key, v := range exchange.GlobalExchangeRegistry {
		if subscriber, ok := v.(entity.Subscriber); ok {
			subscribers = append(subscribers, subscriber)
			logrus.Info("added subscriber for exchange: ", key)
		}
	}

	for _, subscriber := range subscribers {
		err := subscriber.JetstreamEventSubscribe()
		if err != nil {
			util.ContinueOrFatal(err)
		}
	}

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"database": func(ctx context.Context) error {
			cancel()
			return db.Close()
		},
		"nats connection": func(ctx context.Context) error {
			return infrastructure.CloseJetstream(nc)
		},
	})

	<-wait
}
