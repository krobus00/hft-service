package entity

import (
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
)

type KlineSubscription struct {
	ID           string    `db:"id" json:"id"`
	Exchange     string    `db:"exchange" json:"exchange"`
	MarketType   string    `db:"market_type" json:"market_type"`
	Symbol       string    `db:"symbol" json:"symbol"`
	Interval     string    `db:"interval" json:"interval"`
	Payload      string    `db:"payload" json:"payload"`
	MaxKlineData int       `db:"max_kline_data" json:"max_kline_data"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

func (k *KlineSubscription) GetColumn() map[string]string {
	return map[string]string{
		"id":             "id",
		"exchange":       "exchange",
		"market_type":    "market_type",
		"symbol":         "symbol",
		"interval":       "interval",
		"payload":        "payload",
		"max_kline_data": "max_kline_data",
		"created_at":     "created_at",
		"updated_at":     "updated_at",
	}
}

func (k *KlineSubscription) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "created_at", Direction: "DESC"}
}

func (k *KlineSubscription) SearchableFields() []string {
	return []string{"exchange", "market_type", "symbol", "interval"}
}
