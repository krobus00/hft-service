package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/krobus00/hft-service/internal/entity"
)

type KlineSubscriptionRepository struct {
	db *sqlx.DB
}

func NewKlineSubscriptionRepository(db *sqlx.DB) *KlineSubscriptionRepository {
	return &KlineSubscriptionRepository{db: db}
}

func (r *KlineSubscriptionRepository) GetAll(ctx context.Context) ([]entity.KlineSubscription, error) {
	var subscriptions []entity.KlineSubscription
	err := r.db.SelectContext(ctx, &subscriptions, "SELECT * FROM kline_subscriptions order by created_at desc")
	return subscriptions, err
}

func (r *KlineSubscriptionRepository) GetByExchange(ctx context.Context, exchange string) ([]entity.KlineSubscription, error) {
	var subscriptions []entity.KlineSubscription
	err := r.db.SelectContext(ctx, &subscriptions, "SELECT * FROM kline_subscriptions WHERE exchange = $1 order by created_at desc", exchange)
	return subscriptions, err
}

func (r *KlineSubscriptionRepository) GetByExchangeAndMarketType(ctx context.Context, exchange, marketType string) ([]entity.KlineSubscription, error) {
	var subscriptions []entity.KlineSubscription
	err := r.db.SelectContext(ctx, &subscriptions, "SELECT * FROM kline_subscriptions WHERE exchange = $1 AND market_type = $2 order by created_at desc", exchange, marketType)
	return subscriptions, err
}

func (r *KlineSubscriptionRepository) GetLatestUpdatedAtByExchange(ctx context.Context, exchange string) (time.Time, error) {
	var updatedAt sql.NullTime
	err := r.db.GetContext(ctx, &updatedAt, "SELECT MAX(updated_at) FROM kline_subscriptions WHERE exchange = $1", exchange)
	if err != nil {
		return time.Time{}, err
	}

	if !updatedAt.Valid {
		return time.Time{}, nil
	}

	return updatedAt.Time, nil
}

func (r *KlineSubscriptionRepository) GetLatestUpdatedAtByExchangeAndMarketType(ctx context.Context, exchange, marketType string) (time.Time, error) {
	var updatedAt sql.NullTime
	err := r.db.GetContext(ctx, &updatedAt, "SELECT MAX(updated_at) FROM kline_subscriptions WHERE exchange = $1 AND market_type = $2", exchange, marketType)
	if err != nil {
		return time.Time{}, err
	}

	if !updatedAt.Valid {
		return time.Time{}, nil
	}

	return updatedAt.Time, nil
}
