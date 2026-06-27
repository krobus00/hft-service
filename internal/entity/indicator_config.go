package entity

import (
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
)

type IndicatorConfig struct {
	ID         string    `db:"id" json:"id"`
	Exchange   string    `db:"exchange" json:"exchange"`
	MarketType string    `db:"market_type" json:"market_type"`
	Symbol     string    `db:"symbol" json:"symbol"`
	Interval   string    `db:"interval" json:"interval"`
	Indicator  string    `db:"indicator" json:"indicator"`
	OutputName string    `db:"output_name" json:"output_name"`
	Params     string    `db:"params" json:"params"`
	Enabled    bool      `db:"enabled" json:"enabled"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}

func (i *IndicatorConfig) GetColumn() map[string]string {
	return map[string]string{
		"id":          "id",
		"exchange":    "exchange",
		"market_type": "market_type",
		"symbol":      "symbol",
		"interval":    "interval",
		"indicator":   "indicator",
		"output_name": "output_name",
		"enabled":     "enabled",
		"created_at":  "created_at",
		"updated_at":  "updated_at",
	}
}

func (i *IndicatorConfig) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "created_at", Direction: "DESC"}
}

func (i *IndicatorConfig) SearchableFields() []string {
	return []string{"exchange", "market_type", "symbol", "interval", "indicator", "output_name"}
}
