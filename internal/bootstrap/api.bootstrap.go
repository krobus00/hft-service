package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/krobus00/hft-service/internal/config"
	apiHandler "github.com/krobus00/hft-service/internal/handler/api/http"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	apiservice "github.com/krobus00/hft-service/internal/service/api"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func StartAPI(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	apiDB, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["api"])
	util.ContinueOrFatal(err)
	infrastructure.StartPostgresHealthCheck(ctx, apiDB, config.Env.Database["api"].PingInterval)

	marketDataDB, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["market_data"])
	util.ContinueOrFatal(err)
	infrastructure.StartPostgresHealthCheck(ctx, marketDataDB, config.Env.Database["market_data"].PingInterval)

	orderEngineDB, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["order_engine"])
	util.ContinueOrFatal(err)
	infrastructure.StartPostgresHealthCheck(ctx, orderEngineDB, config.Env.Database["order_engine"].PingInterval)

	redisClient, err := infrastructure.NewRedisClient(ctx, config.Env.Redis["api"])
	util.ContinueOrFatal(err)
	if redisClient == nil {
		util.ContinueOrFatal(errors.New("redis api cache_dsn is required"))
	}

	authRepo := repository.NewAPIAuthRepository(apiDB)
	authCache := apiservice.NewAuthCache(redisClient, config.Env.Redis["api"].DefaultCacheDuration)
	authService := apiservice.NewAuthService(authRepo, config.Env.DashboardAuth, authCache)
	dataService := apiservice.NewDataService(apiDB, marketDataDB, orderEngineDB)

	apiHTTPHandler := apiHandler.NewAPIHTTPHandler(authService, dataService, config.Env.DashboardAuth)
	httpMux := http.NewServeMux()
	apiHTTPHandler.Register(httpMux)

	httpPort := fmt.Sprintf(":%s", config.Env.Port["api_http"])
	httpServer := infrastructure.NewHTTPServerWithConfig(infrastructure.HTTPServerConfig{
		Addr:            httpPort,
		ShutdownTimeout: config.Env.GracefulShutdownTimeout,
	}, httpMux)

	go func() {
		err := httpServer.Start()
		if err != nil {
			logrus.Error(err)
		}
	}()
	logrus.Info(fmt.Sprintf("api http server started on %s", httpPort))

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"api database": func(ctx context.Context) error {
			cancel()
			return apiDB.Close()
		},
		"market data database": func(ctx context.Context) error {
			cancel()
			return marketDataDB.Close()
		},
		"order engine database": func(ctx context.Context) error {
			cancel()
			return orderEngineDB.Close()
		},
		"redis": func(ctx context.Context) error {
			if redisClient == nil {
				return nil
			}
			return redisClient.Close()
		},
		"http": func(ctx context.Context) error {
			return httpServer.Shutdown(ctx)
		},
	})

	<-wait
}
