package ordermanager

import (
	"context"

	"github.com/krobus00/hft-service/internal/entity"
)

type OrderManagerService struct {
	exchange entity.Exchange
}

func NewOrderManagerService(exchange entity.Exchange) entity.OrderManager {
	return &OrderManagerService{
		exchange: exchange,
	}
}

func (s *OrderManagerService) PlaceOrder(ctx context.Context, order entity.OrderRequest) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if err := s.exchange.PlaceOrder(ctx, order); err != nil {
		return err
	}

	return nil
}
