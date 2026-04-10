package entity

import "time"

type SymbolMapping struct {
	ID          string    `db:"id" json:"id"`
	Exchange    string    `db:"exchange" json:"exchange"`
	Symbol      string    `db:"symbol" json:"symbol"`
	KlineSymbol string    `db:"kline_symbol" json:"kline_symbol"`
	OrderSymbol string    `db:"order_symbol" json:"order_symbol"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type ExchangeSymbols struct {
	InternalToKline map[string]string
	InternalToOrder map[string]string
	KlineToInternal map[string]string
	OrderToInternal map[string]string
}

// [exchange] = mapping indexes where keys are normalized symbols.
type ExchangeSymbolMapping map[string]ExchangeSymbols
