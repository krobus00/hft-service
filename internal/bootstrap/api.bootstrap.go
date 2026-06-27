package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/krobus00/hft-service/internal/config"
	apiHandler "github.com/krobus00/hft-service/internal/handler/api/http"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	apiservice "github.com/krobus00/hft-service/internal/service/api"
	"github.com/krobus00/hft-service/internal/util"
	pb "github.com/krobus00/hft-service/pb/market_data"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

	nc, js, err := infrastructure.NewJetstream()
	util.ContinueOrFatal(err)

	authRepo := repository.NewAPIAuthRepository(apiDB)
	authCache := apiservice.NewAuthCache(redisClient, config.Env.Redis["api"].DefaultCacheDuration)
	authService := apiservice.NewAuthService(authRepo, config.Env.DashboardAuth, authCache)
	dataService := apiservice.NewDataService(apiDB, marketDataDB, orderEngineDB, js)

	marketDataGRPCAddr := marketDataGatewayGRPCAddress()
	marketDataGRPCConn, err := grpc.NewClient(marketDataGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	util.ContinueOrFatal(err)
	backfillService := apiservice.NewBackfillService(pb.NewMarketDataServiceClient(marketDataGRPCConn), repository.NewAPIBackfillJobRepository(apiDB))

	apiHTTPHandler := apiHandler.NewAPIHTTPHandler(authService, dataService, backfillService, config.Env.DashboardAuth)
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
		"nats connection": func(ctx context.Context) error {
			return infrastructure.CloseJetstream(nc)
		},
		"market data grpc": func(ctx context.Context) error {
			return marketDataGRPCConn.Close()
		},
		"http": func(ctx context.Context) error {
			return httpServer.Shutdown(ctx)
		},
	})

	<-wait
}

func marketDataGatewayGRPCAddress() string {
	if raw := strings.TrimSpace(os.Getenv("MARKET_DATA_GATEWAY_GRPC_ADDR")); raw != "" {
		return raw
	}
	if raw := strings.TrimSpace(config.Env.Port["market_data_gateway_grpc_address"]); raw != "" {
		return raw
	}
	port := strings.TrimSpace(config.Env.Port["market_data_gateway_grpc"])
	if port == "" {
		port = "9802"
	}
	return "localhost:" + port
}
