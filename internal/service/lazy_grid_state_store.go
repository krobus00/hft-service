package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

type LazyGridState struct {
	AnchorPrice  decimal.Decimal    `json:"anchor_price"`
	LastLevel    int                `json:"last_level"`
	Positions    []LazyGridPosition `json:"positions"`
	PendingBuys  []LazyGridPosition `json:"pending_buys,omitempty"`
	PendingSells []LazyGridPosition `json:"pending_sells,omitempty"`
	FilledLevels []int              `json:"filled_levels,omitempty"`
}

type LazyGridPosition struct {
	Level    int             `json:"level"`
	Quantity decimal.Decimal `json:"quantity"`
}

type LazyGridStateStore interface {
	Load(ctx context.Context, key string) (LazyGridState, bool, error)
	Save(ctx context.Context, key string, state LazyGridState) error
}

type RedisLazyGridStateStore struct {
	client *redis.Client
}

func NewRedisLazyGridStateStore(cacheDSN string) (*RedisLazyGridStateStore, error) {
	if cacheDSN == "" {
		return nil, fmt.Errorf("redis cache_dsn is required")
	}

	options, err := redis.ParseURL(cacheDSN)
	if err != nil {
		return nil, fmt.Errorf("parse redis cache_dsn: %w", err)
	}

	return &RedisLazyGridStateStore{client: redis.NewClient(options)}, nil
}

func (s *RedisLazyGridStateStore) Load(ctx context.Context, key string) (LazyGridState, bool, error) {
	rawState, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return LazyGridState{}, false, nil
		}
		return LazyGridState{}, false, err
	}

	var state LazyGridState
	if err := json.Unmarshal([]byte(rawState), &state); err != nil {
		return LazyGridState{}, false, err
	}

	return state, true, nil
}

func (s *RedisLazyGridStateStore) Save(ctx context.Context, key string, state LazyGridState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return s.client.Set(ctx, key, payload, 0).Err()
}

func (s *RedisLazyGridStateStore) Close() error {
	return s.client.Close()
}
