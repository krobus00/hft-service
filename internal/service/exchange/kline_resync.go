package exchange

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krobus00/hft-service/internal/entity"
)

type klineResyncState struct {
	SymbolMappingUpdatedAt      time.Time
	KlineSubscriptionUpdatedAt  time.Time
}

func (s klineResyncState) Equal(other klineResyncState) bool {
	return s.SymbolMappingUpdatedAt.Equal(other.SymbolMappingUpdatedAt) && s.KlineSubscriptionUpdatedAt.Equal(other.KlineSubscriptionUpdatedAt)
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
