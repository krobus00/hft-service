package exchange

import (
	"context"

	"github.com/gorilla/websocket"
	"github.com/krobus00/hft-service/internal/entity"
)

func resyncSymbolMappingAndSubscriptions(
	ctx context.Context,
	conn *websocket.Conn,
	fallback []entity.KlineSubscription,
	refreshSymbolMapping func(context.Context) error,
	loadLatestSubscriptions func(context.Context, []entity.KlineSubscription) ([]entity.KlineSubscription, error),
) ([]entity.KlineSubscription, error) {
	if refreshSymbolMapping != nil {
		if err := refreshSymbolMapping(ctx); err != nil {
			return nil, err
		}
	}

	if conn != nil {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "resync"))
		_ = conn.Close()
	}

	if loadLatestSubscriptions == nil {
		return fallback, nil
	}

	subs, err := loadLatestSubscriptions(ctx, fallback)
	if err != nil {
		return nil, err
	}

	return subs, nil
}
