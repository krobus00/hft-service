package repository

import (
	"strings"
	"testing"
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
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

func TestOrderStateFilterAndSortHappenBeforePagination(t *testing.T) {
	req := &apiutil.PaginationReq{
		Filter:   []apiutil.FilterReq{{Field: "state", Op: "eq", Value: []string{"running"}}},
		Sort:     apiutil.SortReq{Field: "state", Direction: "ASC"},
		Paginate: apiutil.PaginateReq{Limit: 10, Offset: 10},
	}
	query, _, err := orderHistoryMetricsFilteredSelect(req).
		OrderBy(req.Sort.Field + " " + req.Sort.Direction).
		Limit(uint64(req.Paginate.Limit)).
		Offset(uint64(req.Paginate.Offset)).
		ToSql()
	if err != nil {
		t.Fatal(err)
	}
	filterAt := strings.LastIndex(query, "WHERE state =")
	sortAt := strings.LastIndex(query, "ORDER BY state ASC")
	limitAt := strings.LastIndex(query, "LIMIT 10 OFFSET 10")
	if filterAt < 0 || sortAt < filterAt || limitAt < sortAt {
		t.Fatalf("state filter/sort must precede pagination: %s", query)
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

func TestStrategyPerformanceIncludesUnpairedSignalInventory(t *testing.T) {
	query, args := strategyPerformanceQuery(entity.OrderReportFilter{StrategyID: "python-micro-grid"})
	for _, want := range []string{"oh.status = 'FILLED'", "oh.trade_condition = 'SIGNAL'", "LEFT JOIN price_references", "sp.sell_proceeds + sp.inventory_quantity"} {
		if !strings.Contains(query, want) {
			t.Fatalf("strategy performance query missing %q", want)
		}
	}
	if len(args) != 2 || args[0] != "python-micro-grid" || args[1] != "python-micro-grid" {
		t.Fatalf("unexpected strategy filters: %v", args)
	}
}

func TestRepositoryOwnsReportPaginationAndEnumSorting(t *testing.T) {
	page, limit := orderReportPagination(entity.OrderReportFilter{Page: -1, Limit: 1000})
	if page != 1 || limit != 50 {
		t.Fatalf("unexpected pagination defaults: page=%d limit=%d", page, limit)
	}

	got := NormalizeExchangeEnums([]string{" BINANCE", ""}, []string{"BYBIT", "BINANCE"})
	want := []string{"BINANCE", "BYBIT"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected exchange enums: %v", got)
	}
}
