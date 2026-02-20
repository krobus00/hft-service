package ordermanager

import (
	"context"

	"github.com/shopspring/decimal"
)

type ExchangeName string

const (
	ExchangeTokoCrypto ExchangeName = "TOKOCRYPTO"
)

type KlineData struct {
	Close    decimal.Decimal
	IsClosed bool
}

type Exchange interface {
	HandleKlineData(ctx context.Context, message []byte) (KlineData, error)
	PlaceOrder(ctx context.Context, order OrderRequest) error
}
