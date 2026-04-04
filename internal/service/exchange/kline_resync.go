package exchange

import (
	"context"
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
	SymbolMapping     *atomic.Value
	SymbolMappingRepo *repository.SymbolMappingRepository
	KlineSubRepo      *repository.KlineSubscriptionRepository
}

func resolveInternalSymbolFromMapping(deps exchangeKlineResyncDeps, exchangeSymbol string) string {
	normalized := strings.ToUpper(strings.TrimSpace(exchangeSymbol))
	if normalized == "" {
		return ""
	}

	mapping := snapshotSymbolMapping(deps.SymbolMapping)
	for internalSymbol, klineSymbol := range mapping[string(deps.ExchangeName)] {
		if strings.EqualFold(strings.TrimSpace(klineSymbol), normalized) {
			return internalSymbol
		}
	}

	return normalized
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
		return fallback, nil
	}

	subs, err := deps.KlineSubRepo.GetByExchange(ctx, string(deps.ExchangeName))
	if err != nil {
		return nil, err
	}

	return subs, nil
}

func loadExchangeResyncState(ctx context.Context, deps exchangeKlineResyncDeps) (klineResyncState, error) {
	state := klineResyncState{}

	if deps.SymbolMappingRepo != nil {
		timestamp, err := deps.SymbolMappingRepo.GetLatestUpdatedAtByExchange(ctx, string(deps.ExchangeName))
		if err != nil {
			return klineResyncState{}, err
		}
		state.SymbolMappingUpdatedAt = timestamp
	}

	if deps.KlineSubRepo != nil {
		timestamp, err := deps.KlineSubRepo.GetLatestUpdatedAtByExchange(ctx, string(deps.ExchangeName))
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
