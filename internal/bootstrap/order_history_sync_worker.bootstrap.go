package bootstrap

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/service/exchange"
	"github.com/krobus00/hft-service/internal/service/orderengine"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func StartOrderHistorySyncWorker(cmd *cobra.Command, args []string) {
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

	symbolMappingRepo := repository.NewSymbolMappingRepository(marketDataDB)
	marketKlineRepo := repository.NewMarketKlineRepository(marketDataDB)
	orderHistoryRepo := repository.NewOrderHistoryRepository(orderEngineDB)

	exchange.InitTokocryptoExchange(ctx, config.Env.Exchanges[string(entity.ExchangeTokoCrypto)], symbolMappingRepo, js, marketKlineRepo)

	syncInterval := resolveOrderHistorySyncInterval()
	orderHistorySyncService := orderengine.NewOrderHistorySyncService(exchange.GlobalExchangeRegistry, orderHistoryRepo, syncInterval)

	go orderHistorySyncService.Run(ctx)

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"market data database": func(ctx context.Context) error {
			cancel()
			return marketDataDB.Close()
		},
		"order engine database": func(ctx context.Context) error {
			cancel()
			return orderEngineDB.Close()
		},
		"nats connection": func(ctx context.Context) error {
			return infrastructure.CloseJetstream(nc)
		},
	})

	<-wait
}

func resolveOrderHistorySyncInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("ORDER_HISTORY_SYNC_INTERVAL"))
	if raw == "" {
		return 0
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		logrus.WithError(err).WithField("value", raw).Warn("invalid ORDER_HISTORY_SYNC_INTERVAL; using default")
		return 0
	}

	return parsed
}
