package entity

import (
	"context"

	"github.com/shopspring/decimal"
)

type OrderType string
type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"

	OrderTypeLimit  OrderType = "LIMIT"
	OrderTypeMarket OrderType = "MARKET"
)

type OrderRequest struct {
	Exchange string
	Symbol   string
	Type     OrderType
	Side     OrderSide
	Price    decimal.Decimal
	Quantity decimal.Decimal
	Source   string
}

type OrderManager interface {
	PlaceOrder(ctx context.Context, order OrderRequest) error
}
