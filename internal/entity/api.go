package entity

import (
	"database/sql"
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/shopspring/decimal"
)

type APIUser struct {
	ID           string       `db:"id" json:"id"`
	Email        string       `db:"email" json:"email"`
	Name         string       `db:"name" json:"name"`
	PasswordHash string       `db:"password_hash" json:"-"`
	Active       bool         `db:"active" json:"active"`
	LastLoginAt  sql.NullTime `db:"last_login_at" json:"last_login_at,omitempty"`
	CreatedAt    time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time    `db:"updated_at" json:"updated_at"`
}

func (u *APIUser) GetColumn() map[string]string {
	return map[string]string{
		"id":            "id",
		"email":         "email",
		"name":          "name",
		"active":        "active",
		"last_login_at": "last_login_at",
		"created_at":    "created_at",
		"updated_at":    "updated_at",
	}
}

func (u *APIUser) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "created_at", Direction: "DESC"}
}

func (u *APIUser) SearchableFields() []string {
	return []string{"email", "name"}
}

type APIRole struct {
	ID          string    `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

func (r *APIRole) GetColumn() map[string]string {
	return map[string]string{
		"id":          "id",
		"name":        "name",
		"description": "description",
		"created_at":  "created_at",
		"updated_at":  "updated_at",
	}
}

func (r *APIRole) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "name", Direction: "ASC"}
}

func (r *APIRole) SearchableFields() []string {
	return []string{"name", "description"}
}

type APIPermission struct {
	ID          string    `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

type APISetting struct {
	ID        string    `db:"id" json:"id"`
	Key       string    `db:"key" json:"key"`
	Value     string    `db:"value" json:"value"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

func (s *APISetting) GetColumn() map[string]string {
	return map[string]string{
		"id":         "id",
		"key":        "key",
		"value":      "value",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}
}

func (s *APISetting) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "updated_at", Direction: "DESC"}
}

func (s *APISetting) SearchableFields() []string {
	return []string{"key"}
}

type StrategyConfig struct {
	ID                       string          `db:"id" json:"id"`
	Strategy                 string          `db:"strategy" json:"strategy"`
	Exchange                 string          `db:"exchange" json:"exchange"`
	MarketType               string          `db:"market_type" json:"market_type"`
	Symbol                   string          `db:"symbol" json:"symbol"`
	Interval                 string          `db:"interval" json:"interval"`
	UserID                   sql.NullString  `db:"user_id" json:"user_id,omitempty"`
	PositionSide             string          `db:"position_side" json:"position_side"`
	Source                   string          `db:"source" json:"source"`
	NeedNotification         bool            `db:"need_notification" json:"need_notification"`
	IsPaperTrading           bool            `db:"is_paper_trading" json:"is_paper_trading"`
	OrderType                string          `db:"order_type" json:"order_type"`
	OrderQty                 decimal.Decimal `db:"order_qty" json:"order_qty"`
	LimitSlippagePct         decimal.Decimal `db:"limit_slippage_pct" json:"limit_slippage_pct"`
	CooldownBars             int             `db:"cooldown_bars" json:"cooldown_bars"`
	SLCooldownBars           int             `db:"sl_cooldown_bars" json:"sl_cooldown_bars"`
	MaxConsecutiveStopLosses int             `db:"max_consecutive_stop_losses" json:"max_consecutive_stop_losses"`
	SLPauseBars              int             `db:"sl_pause_bars" json:"sl_pause_bars"`
	TakeProfitPct            decimal.Decimal `db:"take_profit_pct" json:"take_profit_pct"`
	StopLossPct              decimal.Decimal `db:"stop_loss_pct" json:"stop_loss_pct"`
	TrailingStopPct          decimal.Decimal `db:"trailing_stop_pct" json:"trailing_stop_pct"`
	TrailingStopTriggerPct   decimal.Decimal `db:"trailing_stop_trigger_pct" json:"trailing_stop_trigger_pct"`
	MaxHoldBars              int             `db:"max_hold_bars" json:"max_hold_bars"`
	MaxPositions             int             `db:"max_positions" json:"max_positions"`
	EnableIntrabarRiskExit   bool            `db:"enable_intrabar_risk_exit" json:"enable_intrabar_risk_exit"`
	CreatedAt                time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt                time.Time       `db:"updated_at" json:"updated_at"`
}

func (s *StrategyConfig) GetColumn() map[string]string {
	return map[string]string{
		"id":                          "id",
		"strategy":                    "strategy",
		"exchange":                    "exchange",
		"market_type":                 "market_type",
		"symbol":                      "symbol",
		"interval":                    "interval",
		"user_id":                     "user_id",
		"position_side":               "position_side",
		"source":                      "source",
		"need_notification":           "need_notification",
		"is_paper_trading":            "is_paper_trading",
		"order_type":                  "order_type",
		"order_qty":                   "order_qty",
		"limit_slippage_pct":          "limit_slippage_pct",
		"cooldown_bars":               "cooldown_bars",
		"sl_cooldown_bars":            "sl_cooldown_bars",
		"max_consecutive_stop_losses": "max_consecutive_stop_losses",
		"sl_pause_bars":               "sl_pause_bars",
		"take_profit_pct":             "take_profit_pct",
		"stop_loss_pct":               "stop_loss_pct",
		"trailing_stop_pct":           "trailing_stop_pct",
		"trailing_stop_trigger_pct":   "trailing_stop_trigger_pct",
		"max_hold_bars":               "max_hold_bars",
		"max_positions":               "max_positions",
		"enable_intrabar_risk_exit":   "enable_intrabar_risk_exit",
		"created_at":                  "created_at",
		"updated_at":                  "updated_at",
	}
}

func (s *StrategyConfig) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "created_at", Direction: "DESC"}
}

func (s *StrategyConfig) SearchableFields() []string {
	return []string{"strategy", "exchange", "market_type", "symbol", "interval", "user_id", "source"}
}
