package api

import (
	"database/sql"
	"testing"

	"github.com/krobus00/hft-service/internal/entity"
	"github.com/shopspring/decimal"
)

func TestManualOrdersKeepStrategyPairing(t *testing.T) {
	entry, err := buildManualEntryOrder("user-1", "manual-entry-1", map[string]any{
		"exchange": "BINANCE", "market_type": "futures", "symbol": "btcusdt", "side": "LONG", "quantity": "2",
	})
	if err != nil || entry.StrategyID == nil || *entry.StrategyID != "manual" || entry.EntryOrderID != entry.RequestID {
		t.Fatalf("invalid manual entry: %#v, %v", entry, err)
	}

	closeOrder, err := buildManualCloseOrder(entity.OrderHistory{
		ID: "history-1", UserID: entry.UserID, Exchange: entry.Exchange, MarketType: entry.MarketType,
		PositionSide: entry.PositionSide, Symbol: entry.Symbol, EntryOrderID: entry.EntryOrderID,
		Side: entry.Side, Quantity: decimal.NewFromInt(2), FilledQuantity: decimal.NewFromInt(2),
		StrategyID: sql.NullString{String: "manual", Valid: true},
	})
	if err != nil || closeOrder.StrategyID == nil || *closeOrder.StrategyID != "manual" || closeOrder.EntryOrderID != entry.EntryOrderID {
		t.Fatalf("close lost strategy pairing: %#v, %v", closeOrder, err)
	}
}

func TestCloneRunningOrderKeepsEntryShape(t *testing.T) {
	order, err := buildCloneRunningOrder(entity.OrderHistoryWithMetrics{OrderHistory: entity.OrderHistory{
		ID: "history-1", UserID: "user-1", Exchange: "binance", MarketType: "futures",
		PositionSide: "LONG", Symbol: "BTC_USDT", Side: entity.OrderSideLong,
		Quantity: decimal.NewFromInt(2), FilledQuantity: decimal.NewFromInt(2),
		StrategyID: sql.NullString{String: "go-ema-cross", Valid: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if order.TradeCondition != string(entity.TradeConditionEntry) || order.Side != entity.OrderSideLong || !order.Quantity.Equal(decimal.NewFromInt(2)) {
		t.Fatalf("invalid clone order: %#v", order)
	}
	if order.StrategyID == nil || *order.StrategyID != "go-ema-cross" || order.EntryOrderID != order.RequestID {
		t.Fatalf("clone lost strategy identity: %#v", order)
	}
}
