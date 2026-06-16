package entity

import (
	"time"

	"github.com/shopspring/decimal"
)

type OrderReportFilter struct {
	StartTime  *time.Time
	EndTime    *time.Time
	StrategyID string
	Symbol     string
	Page       int64
	Limit      int64
}

type OrderTradePnL struct {
	EntryOrderID  string          `db:"entry_order_id" json:"entry_order_id"`
	StrategyID    string          `db:"strategy_id" json:"strategy_id"`
	Symbol        string          `db:"symbol" json:"symbol"`
	Side          string          `db:"side" json:"side"`
	EntryPrice    decimal.Decimal `db:"entry_price" json:"entry_price"`
	ExitPrice     decimal.Decimal `db:"exit_price" json:"exit_price"`
	Quantity      decimal.Decimal `db:"qty" json:"qty"`
	Profit        decimal.Decimal `db:"profit" json:"profit"`
	RunningProfit decimal.Decimal `db:"running_profit" json:"running_profit"`
	EntryTime     time.Time       `db:"entry_time" json:"entry_time"`
	ExitTime      time.Time       `db:"exit_time" json:"exit_time"`
	TotalCount    int64           `db:"total_count" json:"-"`
}

type DailyOrderReport struct {
	TradeDate     time.Time       `db:"trade_date" json:"trade_date"`
	StrategyID    string          `db:"strategy_id" json:"strategy_id"`
	Symbol        string          `db:"symbol" json:"symbol"`
	StartTradeAt  time.Time       `db:"start_trade_at" json:"start_trade_at"`
	EndTradeAt    time.Time       `db:"end_trade_at" json:"end_trade_at"`
	TotalProfit   decimal.Decimal `db:"total_profit" json:"total_profit"`
	WinRate       decimal.Decimal `db:"win_rate" json:"win_rate"`
	AverageSize   decimal.Decimal `db:"avg_size" json:"avg_size"`
	TotalTrades   int64           `db:"total_trades" json:"total_trades"`
	WinningTrades int64           `db:"winning_trades" json:"winning_trades"`
	LosingTrades  int64           `db:"losing_trades" json:"losing_trades"`
}

type StrategyPerformanceReport struct {
	StrategyID    string          `db:"strategy_id" json:"strategy_id"`
	TotalProfit   decimal.Decimal `db:"total_profit" json:"total_profit"`
	WinRate       decimal.Decimal `db:"win_rate" json:"win_rate"`
	AverageProfit decimal.Decimal `db:"avg_profit" json:"avg_profit"`
	AverageSize   decimal.Decimal `db:"avg_size" json:"avg_size"`
	BestTrade     decimal.Decimal `db:"best_trade" json:"best_trade"`
	WorstTrade    decimal.Decimal `db:"worst_trade" json:"worst_trade"`
	ProfitFactor  decimal.Decimal `db:"profit_factor" json:"profit_factor"`
	TotalTrades   int64           `db:"total_trades" json:"total_trades"`
	WinningTrades int64           `db:"winning_trades" json:"winning_trades"`
	LosingTrades  int64           `db:"losing_trades" json:"losing_trades"`
	FirstTradeAt  time.Time       `db:"first_trade_at" json:"first_trade_at"`
	LastTradeAt   time.Time       `db:"last_trade_at" json:"last_trade_at"`
}
