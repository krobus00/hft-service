package entity

import (
	"time"

	"github.com/shopspring/decimal"
)

type KlineData struct {
	Close    decimal.Decimal
	IsClosed bool
}

type MarketKlineEvent struct {
	RetryCount int         `json:"retry"`
	Data       MarketKline `json:"data"`
}

type MarketKline struct {
	Exchange         string
	EventType        string
	EventTime        time.Time
	Symbol           string
	Interval         string
	OpenTime         time.Time
	CloseTime        time.Time
	OpenPrice        decimal.Decimal
	HighPrice        decimal.Decimal
	LowPrice         decimal.Decimal
	ClosePrice       decimal.Decimal
	BaseVolume       decimal.Decimal
	QuoteVolume      decimal.Decimal
	TakerBaseVolume  decimal.Decimal
	TakerQuoteVolume decimal.Decimal
	TradeCount       int32
	IsClosed         bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (m MarketKline) TableName() string {
	return "market_klines"
}
