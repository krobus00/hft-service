package strategy

import (
	"database/sql"
	"testing"

	"github.com/krobus00/hft-service/internal/entity"
	"github.com/shopspring/decimal"
)

func TestBuildExitOrderKeepsEntryPairing(t *testing.T) {
	cfg := entity.StrategyConfig{
		ID: "cfg-1", Strategy: "ema-cross", UserID: sql.NullString{String: "user-1", Valid: true},
		OrderType: string(entity.OrderTypeMarket), NeedNotification: true, IsPaperTrading: true,
	}
	rule := entity.StrategyRule{ID: "rule-1", TradeCondition: string(entity.TradeConditionExit), OrderReason: "signal_exit"}
	entry := entity.OrderHistory{
		ID: "history-1", UserID: "user-1", Exchange: "binance", MarketType: "spot", Symbol: "BTC_USDT",
		PositionSide: string(entity.PositionSideBoth), EntryOrderID: "entry-1", Side: entity.OrderSideBuy,
		Quantity: decimal.NewFromInt(2), FilledQuantity: decimal.NewFromInt(2),
	}

	order, ok := buildExitOrder(cfg, rule, entity.MarketKlineIndicatorEvent{}, entry)
	if !ok {
		t.Fatal("expected exit order")
	}
	if order.EntryOrderID != "entry-1" || order.TradeCondition != string(entity.TradeConditionExit) || order.Side != entity.OrderSideSell {
		t.Fatalf("exit order lost close pairing: %#v", order)
	}
}

func TestBuildEntryOrderKeepsEntryID(t *testing.T) {
	cfg := entity.StrategyConfig{
		ID: "cfg-1", Strategy: "ema-cross", Exchange: "binance", MarketType: "spot", Symbol: "BTC_USDT",
		Interval: "1m", UserID: sql.NullString{String: "user-1", Valid: true},
		OrderType: string(entity.OrderTypeMarket), OrderQty: decimal.NewFromInt(1),
	}
	rule := entity.StrategyRule{ID: "rule-1", Side: string(entity.OrderSideBuy), TradeCondition: string(entity.TradeConditionEntry)}

	order, ok := buildEntryOrder(cfg, rule, entity.MarketKlineIndicatorEvent{})
	if !ok {
		t.Fatal("expected entry order")
	}
	if order.EntryOrderID == "" || order.EntryOrderID != order.RequestID {
		t.Fatalf("entry order must pair to its request: %#v", order)
	}
}
