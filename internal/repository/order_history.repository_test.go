package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/krobus00/hft-service/internal/entity"
)

func TestOrderHistoryMetricsUsesMaterializedTradesAndPriceReferences(t *testing.T) {
	query, _, err := orderHistoryMetricsSelect().ToSql()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"LEFT JOIN order_trades trade", "LEFT JOIN price_references pr", "pr.price - COALESCE", "- pr.price"} {
		if !strings.Contains(query, want) {
			t.Fatalf("metrics query missing %q", want)
		}
	}
	if strings.Contains(query, "LATERAL") {
		t.Fatal("metrics query must not pair order history rows at read time")
	}
}

func TestTradeReportFiltersByRealizationTime(t *testing.T) {
	start := time.Unix(1, 0)
	end := time.Unix(2, 0)
	where, args := tradeReportWhere(entity.OrderReportFilter{StartTime: &start, EndTime: &end})
	if !strings.Contains(where, "exit_time >= $1") || !strings.Contains(where, "exit_time <= $2") {
		t.Fatalf("unexpected trade report filter: %s", where)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 filter arguments, got %d", len(args))
	}
}
