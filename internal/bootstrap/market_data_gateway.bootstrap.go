package bootstrap

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	grpcHandler "github.com/krobus00/hft-service/internal/handler/marketdata/grpc"
	"github.com/krobus00/hft-service/internal/infrastructure"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/service/exchange"
	"github.com/krobus00/hft-service/internal/util"
	pb "github.com/krobus00/hft-service/pb/market_data"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func StartMarketDataGateway(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := infrastructure.NewPostgresConnection(ctx, config.Env.Database["market_data"])
	util.ContinueOrFatal(err)
	infrastructure.StartPostgresHealthCheck(ctx, db, config.Env.Database["market_data"].PingInterval)

	nc, js, err := infrastructure.NewJetstream()
	util.ContinueOrFatal(err)

	klineSubscriptionRepo := repository.NewKlineSubscriptionRepository(db)
	symbolMappingRepo := repository.NewSymbolMappingRepository(db)
	marketKlineRepo := repository.NewMarketKlineRepository(db)

	initConfiguredExchanges(ctx, symbolMappingRepo, klineSubscriptionRepo, js, marketKlineRepo)

	var subscriptionWG sync.WaitGroup

	for key, v := range exchange.GlobalExchangeRegistry {
		subscriptionWG.Add(1)
		go func(key entity.ExchangeName, v entity.Exchange) {
			defer subscriptionWG.Done()
			subs, err := klineSubscriptionRepo.GetByExchange(ctx, string(key))
			if err != nil {
				logrus.Errorf("error getting kline subscriptions for exchange %s: %v", key, err)
				return
			}
			logrus.Info("starting subscription for exchange: ", key)
			err = v.SubscribeKlineData(ctx, subs)
			if err != nil {
				logrus.Errorf("error subscribing to kline data for exchange %s: %v", key, err)
			}
		}(key, v)
	}

	publishers := make([]entity.Publisher, 0)
	for key, v := range exchange.GlobalExchangeRegistry {
		if publisher, ok := v.(entity.Publisher); ok {
			publishers = append(publishers, publisher)
			logrus.Info("added publisher for exchange: ", key)
		}
	}

	for _, publisher := range publishers {
		err := publisher.JetstreamEventInit(ctx)
		if err != nil {
			util.ContinueOrFatal(err)
		}
	}

	grpcServer := grpc.NewServer()
	marketDataGRPCServer := grpcHandler.NewMarketDataGRPCServer(exchange.GlobalExchangeRegistry)
	pb.RegisterMarketDataServiceServer(grpcServer, marketDataGRPCServer)

	if config.Env.Env == constant.DevelopmentEnvironment {
		reflection.Register(grpcServer)
	}

	grpcPort := config.Env.Port["market_data_gateway_grpc"]
	if grpcPort == "" {
		grpcPort = "9802"
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	util.ContinueOrFatal(err)

	go func() {
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			logrus.Errorf("market data grpc server stopped: %v", serveErr)
		}
	}()
	logrus.Infof("market data grpc server started on :%s", grpcPort)

	wait := gracefulShutdown(ctx, config.Env.GracefulShutdownTimeout, map[string]operation{
		"grpc": func(ctx context.Context) error {
			return lis.Close()
		},
		"exchange-ws connection": func(ctx context.Context) error {
			cancel()
			subscriptionWG.Wait()
			return nil
		},
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
