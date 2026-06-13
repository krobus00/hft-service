package entity

import (
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
)

type SymbolMapping struct {
	ID          string    `db:"id" json:"id"`
	Exchange    string    `db:"exchange" json:"exchange"`
	MarketType  string    `db:"market_type" json:"market_type"`
	Symbol      string    `db:"symbol" json:"symbol"`
	KlineSymbol string    `db:"kline_symbol" json:"kline_symbol"`
	OrderSymbol string    `db:"order_symbol" json:"order_symbol"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

func (s *SymbolMapping) GetColumn() map[string]string {
	return map[string]string{
		"id":           "id",
		"exchange":     "exchange",
		"market_type":  "market_type",
		"symbol":       "symbol",
		"kline_symbol": "kline_symbol",
		"order_symbol": "order_symbol",
		"created_at":   "created_at",
		"updated_at":   "updated_at",
	}
}

func (s *SymbolMapping) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "created_at", Direction: "DESC"}
}

func (s *SymbolMapping) SearchableFields() []string {
	return []string{"exchange", "market_type", "symbol", "kline_symbol", "order_symbol"}
}

type ExchangeSymbols struct {
	InternalToKline map[string]string
	InternalToOrder map[string]string
	KlineToInternal map[string]string
	OrderToInternal map[string]string
}

// [exchange][market_type] = mapping indexes where keys are normalized symbols.
type ExchangeSymbolMapping map[string]map[string]ExchangeSymbols
