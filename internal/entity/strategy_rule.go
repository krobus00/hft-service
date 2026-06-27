package entity

import (
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
)

type StrategyRule struct {
	ID               string    `db:"id" json:"id"`
	StrategyConfigID string    `db:"strategy_config_id" json:"strategy_config_id"`
	Name             string    `db:"name" json:"name"`
	Side             string    `db:"side" json:"side"`
	TradeCondition   string    `db:"trade_condition" json:"trade_condition"`
	ExitType         string    `db:"exit_type" json:"exit_type"`
	OrderReason      string    `db:"order_reason" json:"order_reason"`
	Conditions       string    `db:"conditions" json:"conditions"`
	Enabled          bool      `db:"enabled" json:"enabled"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

func (r *StrategyRule) GetColumn() map[string]string {
	return map[string]string{
		"id":                 "id",
		"strategy_config_id": "strategy_config_id",
		"name":               "name",
		"side":               "side",
		"trade_condition":    "trade_condition",
		"exit_type":          "exit_type",
		"order_reason":       "order_reason",
		"enabled":            "enabled",
		"created_at":         "created_at",
		"updated_at":         "updated_at",
	}
}

func (r *StrategyRule) DefaultSort() apiutil.SortReq {
	return apiutil.SortReq{Field: "created_at", Direction: "DESC"}
}

func (r *StrategyRule) SearchableFields() []string {
	return []string{"strategy_config_id", "name", "side", "trade_condition", "order_reason"}
}
