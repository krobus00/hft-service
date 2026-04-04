package bootstrap

import (
	"context"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/service/exchange"
	"github.com/nats-io/nats.go"
)

func initConfiguredExchanges(ctx context.Context, symbolMappingRepo *repository.SymbolMappingRepository, klineSubscriptionRepo *repository.KlineSubscriptionRepository, js nats.JetStreamContext, marketKlineRepo *repository.MarketKlineRepository) {
	exchange.GlobalExchangeRegistry = make(map[entity.ExchangeName]entity.Exchange)

	if exchangeConfig, ok := config.Env.Exchanges[string(entity.ExchangeTokoCrypto)]; ok {
		exchange.InitTokocryptoExchange(ctx, exchangeConfig, symbolMappingRepo, klineSubscriptionRepo, js, marketKlineRepo)
	}

	if exchangeConfig, ok := config.Env.Exchanges[string(entity.ExchangeBinance)]; ok {
		exchange.InitBinanceExchange(ctx, exchangeConfig, symbolMappingRepo, klineSubscriptionRepo, js, marketKlineRepo)
	}
}
