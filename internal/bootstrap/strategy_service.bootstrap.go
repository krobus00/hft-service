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

	db, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["market_data"])
	util.ContinueOrFatal(err)
	infrastructure.StartPostgresHealthCheck(ctx, db, config.Env.Database["market_data"].PingInterval)

	nc, js, err := infrastructure.NewJetstream()
	util.ContinueOrFatal(err)

	service := strategy.NewService(
		js,
		repository.NewStrategyConfigRepository(db),
		repository.NewStrategyRuleRepository(db),
		repository.NewStrategyStateRepository(db),
	)
	util.ContinueOrFatal(service.Subscribe(ctx))

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
