package exchange

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
)

type klineResyncState struct {
	SymbolMappingUpdatedAt     time.Time
	KlineSubscriptionUpdatedAt time.Time
}

func (s klineResyncState) Equal(other klineResyncState) bool {
	return s.SymbolMappingUpdatedAt.Equal(other.SymbolMappingUpdatedAt) && s.KlineSubscriptionUpdatedAt.Equal(other.KlineSubscriptionUpdatedAt)
}

type exchangeKlineResyncDeps struct {
	ExchangeName      entity.ExchangeName
	MarketType        entity.MarketType
	SymbolMapping     *atomic.Value
	SymbolMappingRepo *repository.SymbolMappingRepository
	KlineSubRepo      *repository.KlineSubscriptionRepository
}

func effectiveMarketType(raw entity.MarketType) string {
	return string(entity.NormalizeMarketType(string(raw)))
}

func resolveInternalSymbolFromMapping(deps exchangeKlineResyncDeps, exchangeSymbol string) string {
	return resolveInternalSymbolFromKlineMapping(deps, exchangeSymbol)
}

func resolveInternalSymbolFromKlineMapping(deps exchangeKlineResyncDeps, exchangeSymbol string) string {
	normalized := strings.ToUpper(strings.TrimSpace(exchangeSymbol))
	if normalized == "" {
		return ""
	}

	mapping := snapshotSymbolMapping(deps.SymbolMapping)
	marketTypeMapping, ok := mapping[string(deps.ExchangeName)]
	if !ok {
		return normalized
	}

	indexes, ok := marketTypeMapping[effectiveMarketType(deps.MarketType)]
	if !ok {
		return normalized
	}

	if internalSymbol, ok := indexes.KlineToInternal[normalized]; ok {
		return internalSymbol
	}

	return normalized
}

func resolveInternalSymbolFromOrderMapping(deps exchangeKlineResyncDeps, exchangeSymbol string) string {
	normalized := strings.ToUpper(strings.TrimSpace(exchangeSymbol))
	if normalized == "" {
		return ""
	}

	mapping := snapshotSymbolMapping(deps.SymbolMapping)
	marketTypeMapping, ok := mapping[string(deps.ExchangeName)]
	if !ok {
		return normalized
	}

	indexes, ok := marketTypeMapping[effectiveMarketType(deps.MarketType)]
	if !ok {
		return normalized
	}

	if internalSymbol, ok := indexes.OrderToInternal[normalized]; ok {
		return internalSymbol
	}

	return normalized
}

func resolveExchangeKlineSymbolFromMapping(deps exchangeKlineResyncDeps, internalSymbol string) string {
	normalizedInternal := strings.ToUpper(strings.TrimSpace(internalSymbol))
	if normalizedInternal == "" {
		return ""
	}

	mapping := snapshotSymbolMapping(deps.SymbolMapping)
	marketTypeMapping, ok := mapping[string(deps.ExchangeName)]
	if !ok {
		return normalizedInternal
	}

	indexes, ok := marketTypeMapping[effectiveMarketType(deps.MarketType)]
	if !ok {
		return normalizedInternal
	}

	if exchangeSymbol, ok := indexes.InternalToKline[normalizedInternal]; ok {
		trimmed := strings.TrimSpace(exchangeSymbol)
		if trimmed != "" {
			return trimmed
		}
	}

	return normalizedInternal
}

func resolveExchangeOrderSymbolFromMapping(deps exchangeKlineResyncDeps, internalSymbol string) string {
	normalizedInternal := strings.ToUpper(strings.TrimSpace(internalSymbol))
	if normalizedInternal == "" {
		return ""
	}

	mapping := snapshotSymbolMapping(deps.SymbolMapping)
	marketTypeMapping, ok := mapping[string(deps.ExchangeName)]
	if !ok {
		return normalizedInternal
	}

	indexes, ok := marketTypeMapping[effectiveMarketType(deps.MarketType)]
	if !ok {
		return normalizedInternal
	}

	if exchangeSymbol, ok := indexes.InternalToOrder[normalizedInternal]; ok {
		trimmed := strings.TrimSpace(exchangeSymbol)
		if trimmed != "" {
			return trimmed
		}
	}

	return normalizedInternal
}

func normalizeExchangeSubscriptions(deps exchangeKlineResyncDeps, subs []entity.KlineSubscription) []entity.KlineSubscription {
	if len(subs) == 0 {
		return subs
	}

	normalized := make([]entity.KlineSubscription, 0, len(subs))
	for _, sub := range subs {
		if sub.Exchange != string(deps.ExchangeName) {
			normalized = append(normalized, sub)
			continue
		}
		if string(entity.NormalizeMarketType(sub.MarketType)) != effectiveMarketType(deps.MarketType) {
			continue
		}

		internalSymbol := resolveInternalSymbolFromKlineMapping(deps, sub.Symbol)
		exchangeSymbol := resolveExchangeKlineSymbolFromMapping(deps, internalSymbol)
		interval := strings.TrimSpace(sub.Interval)

		sub.Symbol = internalSymbol
		sub.MarketType = effectiveMarketType(deps.MarketType)
		sub.Payload = buildKlineSubscriptionPayload(exchangeSymbol, interval)
		normalized = append(normalized, sub)
	}

	return normalized
}

func buildKlineSubscriptionPayload(exchangeKlineSymbol, interval string) string {
	params := []string{strings.ToLower(strings.TrimSpace(exchangeKlineSymbol)) + "@kline_" + strings.TrimSpace(interval)}
	payload := map[string]any{
		"method": "SUBSCRIBE",
		"params": params,
		"id":     1,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}

	return string(raw)
}

func snapshotSymbolMapping(symbolMapping *atomic.Value) entity.ExchangeSymbolMapping {
	if symbolMapping == nil {
		return nil
	}

	raw := symbolMapping.Load()
	if raw == nil {
		return nil
	}

	mapping, ok := raw.(entity.ExchangeSymbolMapping)
	if !ok {
		return nil
	}

	return mapping
}

func persistSymbolMapping(symbolMapping *atomic.Value, mapping entity.ExchangeSymbolMapping) {
	if symbolMapping == nil {
		return
	}

	symbolMapping.Store(mapping)
}

func refreshExchangeSymbolMapping(ctx context.Context, deps exchangeKlineResyncDeps) error {
	if deps.SymbolMappingRepo == nil {
		return nil
	}

	mapping, err := deps.SymbolMappingRepo.GetByExchange(ctx, string(deps.ExchangeName))
	if err != nil {
		return err
	}

	persistSymbolMapping(deps.SymbolMapping, mapping)

	return nil
}

func loadExchangeLatestSubscriptions(ctx context.Context, deps exchangeKlineResyncDeps, fallback []entity.KlineSubscription) ([]entity.KlineSubscription, error) {
	if deps.KlineSubRepo == nil {
		return normalizeExchangeSubscriptions(deps, fallback), nil
	}

	subs, err := deps.KlineSubRepo.GetByExchangeAndMarketType(ctx, string(deps.ExchangeName), effectiveMarketType(deps.MarketType))
	if err != nil {
		return nil, err
	}

	return normalizeExchangeSubscriptions(deps, subs), nil
}

func loadExchangeResyncState(ctx context.Context, deps exchangeKlineResyncDeps) (klineResyncState, error) {
	state := klineResyncState{}

	if deps.SymbolMappingRepo != nil {
		timestamp, err := deps.SymbolMappingRepo.GetLatestUpdatedAtByExchangeAndMarketType(ctx, string(deps.ExchangeName), effectiveMarketType(deps.MarketType))
		if err != nil {
			return klineResyncState{}, err
		}
		state.SymbolMappingUpdatedAt = timestamp
	}

	if deps.KlineSubRepo != nil {
		timestamp, err := deps.KlineSubRepo.GetLatestUpdatedAtByExchangeAndMarketType(ctx, string(deps.ExchangeName), effectiveMarketType(deps.MarketType))
		if err != nil {
			return klineResyncState{}, err
		}
		state.KlineSubscriptionUpdatedAt = timestamp
	}

	return state, nil
}

func resyncSymbolMappingAndSubscriptions(
	ctx context.Context,
	conn *websocket.Conn,
	fallback []entity.KlineSubscription,
	refreshSymbolMapping func(context.Context) error,
	loadLatestSubscriptions func(context.Context, []entity.KlineSubscription) ([]entity.KlineSubscription, error),
	loadResyncState func(context.Context) (klineResyncState, error),
) ([]entity.KlineSubscription, klineResyncState, error) {
	if refreshSymbolMapping != nil {
		if err := refreshSymbolMapping(ctx); err != nil {
			return nil, klineResyncState{}, err
		}
	}

	if conn != nil {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "resync"))
		_ = conn.Close()
	}

	if loadLatestSubscriptions == nil {
		if loadResyncState == nil {
			return fallback, klineResyncState{}, nil
		}

		state, err := loadResyncState(ctx)
		if err != nil {
			return nil, klineResyncState{}, err
		}

		return fallback, state, nil
	}

	subs, err := loadLatestSubscriptions(ctx, fallback)
	if err != nil {
		return nil, klineResyncState{}, err
	}

	if loadResyncState == nil {
		return subs, klineResyncState{}, nil
	}

	state, err := loadResyncState(ctx)
	if err != nil {
		return nil, klineResyncState{}, err
	}

	return subs, state, nil
}
