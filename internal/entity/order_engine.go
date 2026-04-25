package entity

import (
	"database/sql"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type OrderType string
type OrderSide string
type PositionSide string
type TradeCondition string

const (
	OrderSideBuy   OrderSide = "BUY"
	OrderSideSell  OrderSide = "SELL"
	OrderSideLong  OrderSide = "LONG"
	OrderSideShort OrderSide = "SHORT"

	OrderTypeLimit  OrderType = "LIMIT"
	OrderTypeMarket OrderType = "MARKET"

	PositionSideBoth  PositionSide = "BOTH"
	PositionSideLong  PositionSide = "LONG"
	PositionSideShort PositionSide = "SHORT"

	TradeConditionEntry        TradeCondition = "ENTRY"
	TradeConditionExit         TradeCondition = "EXIT"
	TradeConditionTakeProfit   TradeCondition = "TAKE_PROFIT"
	TradeConditionStopLoss     TradeCondition = "STOP_LOSS"
	TradeConditionTrailingStop TradeCondition = "TRAILING_STOP"
	TradeConditionSignal       TradeCondition = "SIGNAL"
	TradeConditionUnknown      TradeCondition = "UNKNOWN"
)

func NormalizePositionSide(raw string) PositionSide {
	switch PositionSide(strings.ToUpper(strings.TrimSpace(raw))) {
	case PositionSideLong:
		return PositionSideLong
	case PositionSideShort:
		return PositionSideShort
	default:
		return PositionSideBoth
	}
}

func NormalizeOrderSide(raw string) OrderSide {
	switch OrderSide(strings.ToUpper(strings.TrimSpace(raw))) {
	case OrderSideBuy:
		return OrderSideBuy
	case OrderSideSell:
		return OrderSideSell
	case OrderSideLong:
		return OrderSideLong
	case OrderSideShort:
		return OrderSideShort
	default:
		return ""
	}
}

func NormalizeOrderSideByMarket(raw string, marketType MarketType) OrderSide {
	normalized := NormalizeOrderSide(raw)
	if normalized == "" {
		return ""
	}

	if NormalizeMarketType(string(marketType)) == MarketTypeFutures {
		switch normalized {
		case OrderSideBuy, OrderSideLong:
			return OrderSideLong
		case OrderSideSell, OrderSideShort:
			return OrderSideShort
		default:
			return ""
		}
	}

	if normalized == OrderSideBuy || normalized == OrderSideSell {
		return normalized
	}

	return ""
}

func NormalizeTradeCondition(raw string) TradeCondition {
	switch TradeCondition(strings.ToUpper(strings.TrimSpace(raw))) {
	case TradeConditionEntry:
		return TradeConditionEntry
	case TradeConditionExit:
		return TradeConditionExit
	case TradeConditionTakeProfit:
		return TradeConditionTakeProfit
	case TradeConditionStopLoss:
		return TradeConditionStopLoss
	case TradeConditionTrailingStop:
		return TradeConditionTrailingStop
	case TradeConditionSignal:
		return TradeConditionSignal
	default:
		return TradeConditionUnknown
	}
}

type OrderRequest struct {
	RequestID      string          `json:"request_id"`
	UserID         string          `json:"user_id"`
	OrderID        *string         `json:"order_id"`
	Exchange       string          `json:"exchange"`
	MarketType     string          `json:"market_type"`
	PositionSide   string          `json:"position_side"`
	Symbol         string          `json:"symbol"`
	Type           OrderType       `json:"type"`
	Side           OrderSide       `json:"side"`
	Price          decimal.Decimal `json:"price"`
	Quantity       decimal.Decimal `json:"quantity"`
	RequestedAt    int64           `json:"requested_at"`
	ExpiredAt      *int64          `json:"expired_at,omitempty"`
	Source         string          `json:"source"`
	StrategyID     *string         `json:"strategy_id,omitempty"`
	TradeCondition string          `json:"trade_condition"`
	IsPaperTrading bool            `json:"is_paper_trading"`
}

type OrderRequestEvent struct {
	Data OrderRequest `json:"data"`
}

type OrderHistory struct {
	ID                string           `db:"id" json:"id"`
	RequestID         string           `db:"request_id" json:"request_id"`
	UserID            string           `db:"user_id" json:"user_id"`
	Exchange          string           `db:"exchange" json:"exchange"`
	MarketType        string           `db:"market_type" json:"market_type"`
	PositionSide      string           `db:"position_side" json:"position_side"`
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
	TradeCondition    string           `db:"trade_condition" json:"trade_condition"`
	ErrorMessage      sql.NullString   `db:"error_message" json:"error_message"`
	CreatedAt         time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time        `db:"updated_at" json:"updated_at"`
	IsPaperTrading    bool             `db:"is_paper_trading" json:"is_paper_trading"`
}

func (o OrderHistory) TableName() string {
	return "order_histories"
}
