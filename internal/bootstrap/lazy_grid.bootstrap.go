package bootstrap

import (
	"context"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/service/exchange"
	"github.com/krobus00/hft-service/internal/service/strategy/lazygrid"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/spf13/cobra"
)

func StartLazyGridStrategy(cmd *cobra.Command, args []string) {
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

	lazyGridService, err := lazygrid.NewLazyGridStrategy(ctx, lazygrid.DefaultLazyGridConfig(), nil, js)
	util.ContinueOrFatal(err)

	publishers := make([]entity.Publisher, 0)
	publishers = append(publishers, lazyGridService)
	for _, v := range publishers {
		err = v.JetstreamEventInit()
		util.ContinueOrFatal(err)
	}

	subscribers := make([]entity.Subscriber, 0)
	subscribers = append(subscribers, lazyGridService)
	for _, v := range subscribers {
		err = v.JetstreamEventSubscribe()
		util.ContinueOrFatal(err)
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
