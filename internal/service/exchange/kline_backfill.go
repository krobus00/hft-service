package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/shopspring/decimal"
)

const marketKlineBackfillBatchLimit = 1000

type marketKlineBackfillDeps struct {
	ExchangeName  entity.ExchangeName
	MarketType    entity.MarketType
	BaseURL       string
	KlinePath     string
	HTTPClient    *http.Client
	SymbolMapping *atomic.Value
}

func backfillMarketKlines(ctx context.Context, deps marketKlineBackfillDeps, marketKlineRepo *repository.MarketKlineRepository, req entity.MarketKlineBackfillRequest) (int, error) {
	if marketKlineRepo == nil {
		return 0, fmt.Errorf("market kline repository is not initialized")
	}
	if deps.HTTPClient == nil {
		return 0, fmt.Errorf("http client is not initialized")
	}

	normalizedSymbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	normalizedInterval := strings.TrimSpace(req.Interval)
	if normalizedSymbol == "" {
		return 0, fmt.Errorf("symbol is required")
	}
	if normalizedInterval == "" {
		return 0, fmt.Errorf("interval is required")
	}

	startMs := req.StartTime.UTC().UnixMilli()
	endMs := req.EndTime.UTC().UnixMilli()
	if startMs <= 0 || endMs <= 0 {
		return 0, fmt.Errorf("start_time and end_time must be valid timestamps")
	}
	if endMs <= startMs {
		return 0, fmt.Errorf("end_time must be greater than start_time")
	}

	exchangeSymbol := resolveExchangeKlineSymbol(deps, normalizedSymbol)
	if exchangeSymbol == "" {
		exchangeSymbol = normalizedSymbol
	}

	nextStartMs := startMs
	insertedCount := 0

	for nextStartMs < endMs {
		rows, err := fetchKlineRows(ctx, deps, exchangeSymbol, normalizedInterval, nextStartMs, endMs)
		if err != nil {
			return insertedCount, err
		}
		if len(rows) == 0 {
			break
		}

		lastOpenTime := int64(0)
		now := time.Now().UTC()
		for _, row := range rows {
			kline, err := buildMarketKlineFromRow(deps.ExchangeName, deps.MarketType, normalizedSymbol, normalizedInterval, row, now)
			if err != nil {
				return insertedCount, err
			}

			openTimeMs := kline.OpenTime.UTC().UnixMilli()
			if openTimeMs >= endMs {
				continue
			}
			if openTimeMs < startMs {
				continue
			}

			if err := marketKlineRepo.Create(ctx, &kline); err != nil {
				return insertedCount, err
			}
			insertedCount++

			if openTimeMs > lastOpenTime {
				lastOpenTime = openTimeMs
			}
		}

		if len(rows) < marketKlineBackfillBatchLimit {
			break
		}
		if lastOpenTime <= 0 {
			break
		}

		nextStartMs = lastOpenTime + 1
	}

	return insertedCount, nil
}

func resolveExchangeKlineSymbol(deps marketKlineBackfillDeps, internalSymbol string) string {
	mapping := snapshotSymbolMapping(deps.SymbolMapping)
	if mapping == nil {
		return internalSymbol
	}

	marketTypeMapping, ok := mapping[string(deps.ExchangeName)]
	if !ok {
		return internalSymbol
	}

	indexes, ok := marketTypeMapping[effectiveMarketType(deps.MarketType)]
	if !ok {
		return internalSymbol
	}

	if symbol, ok := indexes.InternalToKline[internalSymbol]; ok && strings.TrimSpace(symbol) != "" {
		return strings.ToUpper(strings.TrimSpace(symbol))
	}

	return internalSymbol
}

func fetchKlineRows(ctx context.Context, deps marketKlineBackfillDeps, symbol, interval string, startMs, endMs int64) ([][]any, error) {
	query := url.Values{}
	query.Set("symbol", symbol)
	query.Set("interval", interval)
	query.Set("startTime", strconv.FormatInt(startMs, 10))
	query.Set("endTime", strconv.FormatInt(endMs-1, 10))
	query.Set("limit", strconv.Itoa(marketKlineBackfillBatchLimit))

	endpoint := strings.TrimRight(deps.BaseURL, "/") + deps.KlinePath + "?" + query.Encode()

	fmt.Println(endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := deps.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s kline backfill request failed: status=%d body=%s", deps.ExchangeName, resp.StatusCode, string(body))
	}

	var rows [][]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("%s kline backfill response parse failed: %w", deps.ExchangeName, err)
	}

	return rows, nil
}

func buildMarketKlineFromRow(exchange entity.ExchangeName, marketType entity.MarketType, symbol, interval string, row []any, now time.Time) (entity.MarketKline, error) {
	if len(row) < 11 {
		return entity.MarketKline{}, fmt.Errorf("%s kline row has invalid length: %d", exchange, len(row))
	}

	openTimeMs, err := asInt64(row[0])
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid open_time: %w", exchange, err)
	}
	openPrice, err := decimal.NewFromString(asString(row[1]))
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid open_price: %w", exchange, err)
	}
	highPrice, err := decimal.NewFromString(asString(row[2]))
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid high_price: %w", exchange, err)
	}
	lowPrice, err := decimal.NewFromString(asString(row[3]))
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid low_price: %w", exchange, err)
	}
	closePrice, err := decimal.NewFromString(asString(row[4]))
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid close_price: %w", exchange, err)
	}
	baseVolume, err := decimal.NewFromString(asString(row[5]))
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid base_volume: %w", exchange, err)
	}
	closeTimeMs, err := asInt64(row[6])
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid close_time: %w", exchange, err)
	}
	quoteVolume, err := decimal.NewFromString(asString(row[7]))
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid quote_volume: %w", exchange, err)
	}
	tradeCount, err := asInt32(row[8])
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid trade_count: %w", exchange, err)
	}
	takerBaseVolume, err := decimal.NewFromString(asString(row[9]))
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid taker_base_volume: %w", exchange, err)
	}
	takerQuoteVolume, err := decimal.NewFromString(asString(row[10]))
	if err != nil {
		return entity.MarketKline{}, fmt.Errorf("%s invalid taker_quote_volume: %w", exchange, err)
	}

	openAt := time.UnixMilli(openTimeMs).UTC()
	closeAt := time.UnixMilli(closeTimeMs).UTC()

	return entity.MarketKline{
		Exchange:         string(exchange),
		MarketType:       effectiveMarketType(marketType),
		EventType:        "kline",
		EventTime:        closeAt,
		Symbol:           symbol,
		Interval:         interval,
		OpenTime:         openAt,
		CloseTime:        closeAt,
		OpenPrice:        openPrice,
		HighPrice:        highPrice,
		LowPrice:         lowPrice,
		ClosePrice:       closePrice,
		BaseVolume:       baseVolume,
		QuoteVolume:      quoteVolume,
		TakerBaseVolume:  takerBaseVolume,
		TakerQuoteVolume: takerQuoteVolume,
		TradeCount:       tradeCount,
		IsClosed:         true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func asInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int32:
		return int64(v), nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected type %T", value)
	}
}

func asInt32(value any) (int32, error) {
	parsed, err := asInt64(value)
	if err != nil {
		return 0, err
	}

	return int32(parsed), nil
}
