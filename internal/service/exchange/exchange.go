package exchange

import (
	"time"

	"github.com/krobus00/hft-service/internal/entity"
)

var (
	GlobalExchangeRegistry = make(map[entity.ExchangeName]entity.Exchange)
)

const (
	wsReconnectMinDelay = 1 * time.Second
	wsReconnectMaxDelay = 15 * time.Second
	wsReconnectFactor   = 2.0
	wsDialTimeout       = 12 * time.Second
	wsResyncInterval    = 30 * time.Second
	wsPingInterval      = 2 * time.Minute
)

func RegisterExchange(name entity.ExchangeName, exchange entity.Exchange) {
	GlobalExchangeRegistry[name] = exchange
}
