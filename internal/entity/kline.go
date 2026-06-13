package entity

import (
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/shopspring/decimal"
)

type KlineData struct {
	Close    decimal.Decimal
	IsClosed bool
}

type MarketKlineBackfillRequest struct {
	MarketType MarketType
	Symbol     string
	Interval   string
	StartTime  time.Time
	EndTime    time.Time
}

type MarketKlineEvent struct {
	Data MarketKline `json:"data"`
}

type MarketKline struct {
	ID               string          `db:"id" json:"id"`
	Exchange         string          `db:"exchange" json:"exchange"`
	MarketType       string          `db:"market_type" json:"market_type"`
	EventType        string          `db:"event_type" json:"event_type"`
	EventTime        time.Time       `db:"event_time" json:"event_time"`
	Symbol           string          `db:"symbol" json:"symbol"`
	Interval         string          `db:"interval" json:"interval"`
	OpenTime         time.Time       `db:"open_time" json:"open_time"`
	CloseTime        time.Time       `db:"close_time" json:"close_time"`
	OpenPrice        decimal.Decimal `db:"open_price" json:"open_price"`
	HighPrice        decimal.Decimal `db:"high_price" json:"high_price"`
	LowPrice         decimal.Decimal `db:"low_price" json:"low_price"`
	ClosePrice       decimal.Decimal `db:"close_price" json:"close_price"`
	BaseVolume       decimal.Decimal `db:"base_volume" json:"base_volume"`
	QuoteVolume      decimal.Decimal `db:"quote_volume" json:"quote_volume"`
	TakerBaseVolume  decimal.Decimal `db:"taker_base_volume" json:"taker_base_volume"`
	TakerQuoteVolume decimal.Decimal `db:"taker_quote_volume" json:"taker_quote_volume"`
	TradeCount       int32           `db:"trade_count" json:"trade_count"`
	IsClosed         bool            `db:"is_closed" json:"is_closed"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
}

func (m MarketKline) TableName() string {
	return "market_klines"
}

func (m *MarketKline) GetColumn() map[string]string {
	return map[string]string{
		"id":          "id",
		"exchange":    "exchange",
		"market_type": "market_type",
		"event_type":  "event_type",
		"symbol":      "symbol",
		"interval":    "interval",
		"open_time":   "open_time",
		"close_time":  "close_time",
		"is_closed":   "is_closed",
		"created_at":  "created_at",
		"updated_at":  "updated_at",
	}
}

func (m *MarketKline) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "open_time", Direction: "DESC"}
}

func (m *MarketKline) SearchableFields() []string {
	return []string{"exchange", "symbol", "interval", "market_type"}
}
