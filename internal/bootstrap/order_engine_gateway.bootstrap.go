package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	grpcHandler "github.com/krobus00/hft-service/internal/handler/orderengine/grpc"
	httpHandler "github.com/krobus00/hft-service/internal/handler/orderengine/http"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/service/exchange"
	"github.com/krobus00/hft-service/internal/service/orderengine"
	"github.com/krobus00/hft-service/internal/util"
	pb "github.com/krobus00/hft-service/pb/order_engine"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func StartOrderEngineGateway(cmd *cobra.Command, args []string) {
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

	orderEngineService := orderengine.NewOrderEngineService(exchange.GlobalExchangeRegistry, orderHistoryRepo, js)

	publishers := make([]entity.Publisher, 0)
	publishers = append(publishers, orderEngineService)
	for _, v := range publishers {
		err = v.JetstreamEventInit()
		util.ContinueOrFatal(err)
	}

	subscribers := make([]entity.Subscriber, 0)
	subscribers = append(subscribers, orderEngineService)
	for _, v := range subscribers {
		err = v.JetstreamEventSubscribe()
		util.ContinueOrFatal(err)
	}

	grpcServer := grpc.NewServer()
	orderEngineGrpcServer := grpcHandler.NewOrderEngineGRPCServer(orderEngineService)
	pb.RegisterOrderEngineServiceServer(grpcServer, orderEngineGrpcServer)

	if config.Env.Env == constant.DevelopmentEnvironment {
		reflection.Register(grpcServer)
	}

	grpcPort := fmt.Sprintf(":%s", config.Env.Port["order_engine_gateway_grpc"])

	lis, err := net.Listen("tcp", grpcPort)
	util.ContinueOrFatal(err)

	go func() {
		_ = grpcServer.Serve(lis)
	}()
	logrus.Info(fmt.Sprintf("grpc server started on %s", grpcPort))

	orderEngineHTTPHandler := httpHandler.NewOrderEngineHTTPHandler(orderEngineService)
	httpMux := http.NewServeMux()
	orderEngineHTTPHandler.Register(httpMux)

	httpPort := fmt.Sprintf(":%s", config.Env.Port["order_engine_gateway_http"])
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
	logrus.Info(fmt.Sprintf("http server started on %s", httpPort))

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"market data database": func(ctx context.Context) error {
			cancel()
			return marketDataDB.Close()
		},
		"order engine database": func(ctx context.Context) error {
			cancel()
			return orderEngineDB.Close()
		},
		"grpc": func(ctx context.Context) error {
			return lis.Close()
		},
		"http": func(ctx context.Context) error {
			return httpServer.Shutdown(ctx)
		},
		"nats connection": func(ctx context.Context) error {
			return infrastructure.CloseJetstream(nc)
		},
	})

	<-wait
}
