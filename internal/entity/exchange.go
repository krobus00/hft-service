package entity

import (
	"context"
)

type ExchangeName string

const (
	ExchangeTokoCrypto ExchangeName = "tokocrypto"
)

type Exchange interface {
	HandleKlineData(ctx context.Context, message []byte) error
	SubscribeKlineData(ctx context.Context, subscriptions []KlineSubscription) error
	PlaceOrder(ctx context.Context, order OrderRequest) (*OrderHistory, error)
}
