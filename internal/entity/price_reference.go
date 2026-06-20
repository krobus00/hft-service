package entity

import (
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/shopspring/decimal"
)

type PriceReference struct {
	ID         string          `db:"id" json:"id"`
	Exchange   string          `db:"exchange" json:"exchange"`
	MarketType string          `db:"market_type" json:"market_type"`
	Symbol     string          `db:"symbol" json:"symbol"`
	Price      decimal.Decimal `db:"price" json:"price"`
	EventTime  time.Time       `db:"event_time" json:"event_time"`
	UpdatedAt  time.Time       `db:"updated_at" json:"updated_at"`
}

func (p *PriceReference) GetColumn() map[string]string {
	return map[string]string{
		"id":          "id",
		"exchange":    "exchange",
		"market_type": "market_type",
		"symbol":      "symbol",
		"price":       "price",
		"event_time":  "event_time",
		"updated_at":  "updated_at",
	}
}

func (p *PriceReference) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "updated_at", Direction: "DESC"}
}

func (p *PriceReference) SearchableFields() []string {
	return []string{"exchange", "market_type", "symbol"}
}
