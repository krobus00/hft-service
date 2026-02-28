package entity

import (
	"database/sql"
	"time"

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
	RequestID      string          `json:"request_id"`
	UserID         string          `json:"user_id"`
	OrderID        *string         `json:"order_id"`
	Exchange       string          `json:"exchange"`
	Symbol         string          `json:"symbol"`
	Type           OrderType       `json:"type"`
	Side           OrderSide       `json:"side"`
	Price          decimal.Decimal `json:"price"`
	Quantity       decimal.Decimal `json:"quantity"`
	RequestedAt    int64           `json:"requested_at"`
	ExpiredAt      *int64          `json:"expired_at,omitempty"`
	Source         string          `json:"source"`
	StrategyID     *string         `json:"strategy_id,omitempty"`
	IsPaperTrading bool            `json:"is_paper_trading"`
}

type OrderRequestEvent struct {
	RetryCount int          `json:"retry"`
	Data       OrderRequest `json:"data"`
}

type OrderHistory struct {
	ID                string           `db:"id" json:"id"`
	RequestID         string           `db:"request_id" json:"request_id"`
	UserID            string           `db:"user_id" json:"user_id"`
	Exchange          string           `db:"exchange" json:"exchange"`
	Symbol            string           `db:"symbol" json:"symbol"`
	OrderID           string           `db:"order_id" json:"order_id"`
	ClientOrderID     sql.NullString   `db:"client_order_id" json:"client_order_id"`
	Side              OrderSide        `db:"side" json:"side"`
	Type              OrderType        `db:"type" json:"type"`
	Price             *decimal.Decimal `db:"price" json:"price"`
	Quantity          decimal.Decimal  `db:"quantity" json:"quantity"`
	FilledQuantity    decimal.Decimal  `db:"filled_quantity" json:"filled_quantity"`
	AvgFillPrice      *decimal.Decimal `db:"avg_fill_price" json:"avg_fill_price"`
	Status            string           `db:"status" json:"status"`
	Leverage          *decimal.Decimal `db:"leverage" json:"leverage"`
	Fee               *decimal.Decimal `db:"fee" json:"fee"`
	RealizedPnl       *decimal.Decimal `db:"realized_pnl" json:"realized_pnl"`
	CreatedAtExchange sql.NullTime     `db:"created_at_exchange" json:"created_at_exchange"`
	SentAt            sql.NullTime     `db:"sent_at" json:"sent_at"`
	AcknowledgedAt    sql.NullTime     `db:"acknowledged_at" json:"acknowledged_at"`
	FilledAt          sql.NullTime     `db:"filled_at" json:"filled_at"`
	StrategyID        sql.NullString   `db:"strategy_id" json:"strategy_id"`
	ErrorMessage      sql.NullString   `db:"error_message" json:"error_message"`
	CreatedAt         time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time        `db:"updated_at" json:"updated_at"`
	IsPaperTrading    bool             `db:"is_paper_trading" json:"is_paper_trading"`
}

func (o OrderHistory) TableName() string {
	return "order_histories"
}
