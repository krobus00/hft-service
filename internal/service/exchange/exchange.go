package exchange

import "github.com/krobus00/hft-service/internal/entity"

var (
	GlobalExchangeRegistry = make(map[entity.ExchangeName]entity.Exchange)
)

func RegisterExchange(name entity.ExchangeName, exchange entity.Exchange) {
	GlobalExchangeRegistry[name] = exchange
}
