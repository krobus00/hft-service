package repository

import (
	"context"
	"database/sql"
)

type StrategyStateRepository struct {
	db dbRunner
}

type dbRunner interface {
	GetContext(context.Context, any, string, ...any) error
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func NewStrategyStateRepository(db dbRunner) *StrategyStateRepository {
	return &StrategyStateRepository{db: db}
}

func (r *StrategyStateRepository) FindValues(ctx context.Context, exchange, marketType, symbol, interval string) (string, error) {
	var values []byte
	err := r.db.GetContext(ctx, &values, `
SELECT values
FROM strategy_executor_states
WHERE lower(exchange) = lower($1)
  AND lower(market_type) = lower($2)
  AND symbol = $3
  AND interval = $4
LIMIT 1`, exchange, marketType, symbol, interval)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return string(values), nil
}

func (r *StrategyStateRepository) UpsertValues(ctx context.Context, exchange, marketType, symbol, interval, values string) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO strategy_executor_states (exchange, market_type, symbol, interval, values, updated_at)
VALUES ($1, $2, $3, $4, $5::jsonb, NOW())
ON CONFLICT (exchange, market_type, symbol, interval)
DO UPDATE SET values = EXCLUDED.values, updated_at = NOW()`, exchange, marketType, symbol, interval, values)
	return err
}
