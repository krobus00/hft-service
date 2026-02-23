package orderengine

import (
	"context"
	"time"

	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/sirupsen/logrus"
)

const defaultOrderHistorySyncInterval = 30 * time.Second

var pendingOrderHistoryStatuses = []string{"NEW", "PARTIAL"}

type OrderHistorySyncService struct {
	exchanges        map[entity.ExchangeName]entity.Exchange
	orderHistoryRepo *repository.OrderHistoryRepository
	syncInterval     time.Duration
}

func NewOrderHistorySyncService(exchanges map[entity.ExchangeName]entity.Exchange, orderHistoryRepo *repository.OrderHistoryRepository, syncInterval time.Duration) *OrderHistorySyncService {
	if syncInterval <= 0 {
		syncInterval = defaultOrderHistorySyncInterval
	}

	return &OrderHistorySyncService{
		exchanges:        exchanges,
		orderHistoryRepo: orderHistoryRepo,
		syncInterval:     syncInterval,
	}
}

func (s *OrderHistorySyncService) Run(ctx context.Context) {
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	s.SyncPendingOrderHistories(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.SyncPendingOrderHistories(ctx)
		}
	}
}

func (s *OrderHistorySyncService) SyncPendingOrderHistories(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	orderHistories, err := s.orderHistoryRepo.GetByStatus(ctx, pendingOrderHistoryStatuses)
	if err != nil {
		logrus.WithError(err).Error("failed to load order histories to sync")
		return
	}

	for _, history := range orderHistories {
		if ctx.Err() != nil {
			return
		}

		exchange, ok := s.exchanges[entity.ExchangeName(history.Exchange)]
		if !ok {
			logrus.WithField("exchange", history.Exchange).Warn("exchange not found for order history sync")
			continue
		}

		updatedHistory, err := exchange.SyncOrderHistory(ctx, history)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"exchange": history.Exchange,
				"order_id": history.OrderID,
				"status":   history.Status,
			}).WithError(err).Error("failed to sync order history")
			continue
		}
		if updatedHistory == nil {
			continue
		}

		if updatedHistory.ID == "" {
			updatedHistory.ID = history.ID
		}
		if updatedHistory.RequestID == "" {
			updatedHistory.RequestID = history.RequestID
		}
		if updatedHistory.UpdatedAt.IsZero() {
			updatedHistory.UpdatedAt = time.Now().UTC()
		}

		err = s.orderHistoryRepo.Update(ctx, updatedHistory)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"exchange": history.Exchange,
				"order_id": history.OrderID,
			}).WithError(err).Error("failed to update order history")
			continue
		}
	}
}
