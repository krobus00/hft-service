package api

import (
	"database/sql"
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/shopspring/decimal"
)

type APIUserResponse struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Active      bool       `json:"active"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type OrderHistoryResponse struct {
	ID                string           `json:"id"`
	RequestID         string           `json:"request_id"`
	UserID            string           `json:"user_id"`
	Exchange          string           `json:"exchange"`
	MarketType        string           `json:"market_type"`
	PositionSide      string           `json:"position_side"`
	Symbol            string           `json:"symbol"`
	OrderID           string           `json:"order_id"`
	EntryOrderID      string           `json:"entry_order_id"`
	ClientOrderID     *string          `json:"client_order_id,omitempty"`
	Side              entity.OrderSide `json:"side"`
	Type              entity.OrderType `json:"type"`
	Price             *decimal.Decimal `json:"price"`
	Quantity          decimal.Decimal  `json:"quantity"`
	FilledQuantity    decimal.Decimal  `json:"filled_quantity"`
	AvgFillPrice      *decimal.Decimal `json:"avg_fill_price"`
	Status            string           `json:"status"`
	Leverage          *decimal.Decimal `json:"leverage"`
	Fee               *decimal.Decimal `json:"fee"`
	RealizedPnl       *decimal.Decimal `json:"realized_pnl"`
	CreatedAtExchange *time.Time       `json:"created_at_exchange,omitempty"`
	SentAt            *time.Time       `json:"sent_at,omitempty"`
	AcknowledgedAt    *time.Time       `json:"acknowledged_at,omitempty"`
	FilledAt          *time.Time       `json:"filled_at,omitempty"`
	StrategyID        *string          `json:"strategy_id,omitempty"`
	TradeCondition    string           `json:"trade_condition"`
	OrderReason       string           `json:"order_reason"`
	ExitType          string           `json:"exit_type"`
	ErrorMessage      *string          `json:"error_message,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	IsPaperTrading    bool             `json:"is_paper_trading"`
}

type StrategyConfigResponse struct {
	ID                       string          `json:"id"`
	Strategy                 string          `json:"strategy"`
	Exchange                 string          `json:"exchange"`
	MarketType               string          `json:"market_type"`
	Symbol                   string          `json:"symbol"`
	Interval                 string          `json:"interval"`
	UserID                   *string         `json:"user_id,omitempty"`
	PositionSide             string          `json:"position_side"`
	Source                   string          `json:"source"`
	NeedNotification         bool            `json:"need_notification"`
	IsPaperTrading           bool            `json:"is_paper_trading"`
	OrderType                string          `json:"order_type"`
	OrderQty                 decimal.Decimal `json:"order_qty"`
	LimitSlippagePct         decimal.Decimal `json:"limit_slippage_pct"`
	CooldownBars             int             `json:"cooldown_bars"`
	SLCooldownBars           int             `json:"sl_cooldown_bars"`
	MaxConsecutiveStopLosses int             `json:"max_consecutive_stop_losses"`
	SLPauseBars              int             `json:"sl_pause_bars"`
	TakeProfitPct            decimal.Decimal `json:"take_profit_pct"`
	StopLossPct              decimal.Decimal `json:"stop_loss_pct"`
	TrailingStopPct          decimal.Decimal `json:"trailing_stop_pct"`
	TrailingStopTriggerPct   decimal.Decimal `json:"trailing_stop_trigger_pct"`
	MaxHoldBars              int             `json:"max_hold_bars"`
	MaxPositions             int             `json:"max_positions"`
	EnableIntrabarRiskExit   bool            `json:"enable_intrabar_risk_exit"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

func apiUserResponse(user *entity.APIUser) *APIUserResponse {
	if user == nil {
		return nil
	}
	return &APIUserResponse{
		ID:          user.ID,
		Email:       user.Email,
		Name:        user.Name,
		Active:      user.Active,
		LastLoginAt: nullTimePtr(user.LastLoginAt),
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}

func orderHistoryResponse(order *entity.OrderHistory) *OrderHistoryResponse {
	if order == nil {
		return nil
	}
	return &OrderHistoryResponse{
		ID:                order.ID,
		RequestID:         order.RequestID,
		UserID:            order.UserID,
		Exchange:          order.Exchange,
		MarketType:        order.MarketType,
		PositionSide:      order.PositionSide,
		Symbol:            order.Symbol,
		OrderID:           order.OrderID,
		EntryOrderID:      order.EntryOrderID,
		ClientOrderID:     nullStringPtr(order.ClientOrderID),
		Side:              order.Side,
		Type:              order.Type,
		Price:             order.Price,
		Quantity:          order.Quantity,
		FilledQuantity:    order.FilledQuantity,
		AvgFillPrice:      order.AvgFillPrice,
		Status:            order.Status,
		Leverage:          order.Leverage,
		Fee:               order.Fee,
		RealizedPnl:       order.RealizedPnl,
		CreatedAtExchange: nullTimePtr(order.CreatedAtExchange),
		SentAt:            nullTimePtr(order.SentAt),
		AcknowledgedAt:    nullTimePtr(order.AcknowledgedAt),
		FilledAt:          nullTimePtr(order.FilledAt),
		StrategyID:        nullStringPtr(order.StrategyID),
		TradeCondition:    order.TradeCondition,
		OrderReason:       order.OrderReason,
		ExitType:          order.ExitType,
		ErrorMessage:      nullStringPtr(order.ErrorMessage),
		CreatedAt:         order.CreatedAt,
		UpdatedAt:         order.UpdatedAt,
		IsPaperTrading:    order.IsPaperTrading,
	}
}

func strategyConfigResponse(config *entity.StrategyConfig) *StrategyConfigResponse {
	if config == nil {
		return nil
	}
	return &StrategyConfigResponse{
		ID:                       config.ID,
		Strategy:                 config.Strategy,
		Exchange:                 config.Exchange,
		MarketType:               config.MarketType,
		Symbol:                   config.Symbol,
		Interval:                 config.Interval,
		UserID:                   nullStringPtr(config.UserID),
		PositionSide:             config.PositionSide,
		Source:                   config.Source,
		NeedNotification:         config.NeedNotification,
		IsPaperTrading:           config.IsPaperTrading,
		OrderType:                config.OrderType,
		OrderQty:                 config.OrderQty,
		LimitSlippagePct:         config.LimitSlippagePct,
		CooldownBars:             config.CooldownBars,
		SLCooldownBars:           config.SLCooldownBars,
		MaxConsecutiveStopLosses: config.MaxConsecutiveStopLosses,
		SLPauseBars:              config.SLPauseBars,
		TakeProfitPct:            config.TakeProfitPct,
		StopLossPct:              config.StopLossPct,
		TrailingStopPct:          config.TrailingStopPct,
		TrailingStopTriggerPct:   config.TrailingStopTriggerPct,
		MaxHoldBars:              config.MaxHoldBars,
		MaxPositions:             config.MaxPositions,
		EnableIntrabarRiskExit:   config.EnableIntrabarRiskExit,
		CreatedAt:                config.CreatedAt,
		UpdatedAt:                config.UpdatedAt,
	}
}

func apiUserPaginationResponse(resp *apiutil.PaginationResp) *apiutil.PaginationResp {
	items, ok := resp.Items.([]entity.APIUser)
	if !ok {
		return resp
	}
	result := make([]APIUserResponse, 0, len(items))
	for i := range items {
		result = append(result, *apiUserResponse(&items[i]))
	}
	resp.Items = result
	return resp
}

func orderHistoryPaginationResponse(resp *apiutil.PaginationResp) *apiutil.PaginationResp {
	items, ok := resp.Items.([]entity.OrderHistory)
	if !ok {
		return resp
	}
	result := make([]OrderHistoryResponse, 0, len(items))
	for i := range items {
		result = append(result, *orderHistoryResponse(&items[i]))
	}
	resp.Items = result
	return resp
}

func strategyConfigPaginationResponse(resp *apiutil.PaginationResp) *apiutil.PaginationResp {
	items, ok := resp.Items.([]entity.StrategyConfig)
	if !ok {
		return resp
	}
	result := make([]StrategyConfigResponse, 0, len(items))
	for i := range items {
		result = append(result, *strategyConfigResponse(&items[i]))
	}
	resp.Items = result
	return resp
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
