package bootstrap

import (
	"context"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/service/strategy"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/spf13/cobra"
)

func StartStrategyService(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	marketDataDB, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["market_data"])
	util.ContinueOrFatal(err)
	infrastructure.StartPostgresHealthCheck(ctx, marketDataDB, config.Env.Database["market_data"].PingInterval)

	orderEngineDB, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["order_engine"])
	util.ContinueOrFatal(err)
	infrastructure.StartPostgresHealthCheck(ctx, orderEngineDB, config.Env.Database["order_engine"].PingInterval)

	nc, js, err := infrastructure.NewJetstream()
	util.ContinueOrFatal(err)

	service := strategy.NewService(
		js,
		repository.NewStrategyConfigRepository(marketDataDB),
		repository.NewStrategyRuleRepository(marketDataDB),
		repository.NewStrategyStateRepository(marketDataDB),
		repository.NewOrderHistoryRepository(orderEngineDB),
	)
	util.ContinueOrFatal(service.Subscribe(ctx))
	service.StartPeriodicSync(ctx)

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"market data database": func(ctx context.Context) error {
			cancel()
			return marketDataDB.Close()
		},
		"order engine database": func(ctx context.Context) error {
			return orderEngineDB.Close()
		},
		"nats connection": func(ctx context.Context) error {
			return infrastructure.CloseJetstream(nc)
		},
	})

	<-wait
}
