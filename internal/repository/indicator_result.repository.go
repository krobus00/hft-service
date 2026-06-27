package repository

import (
	"context"
	"encoding/json"
)

type IndicatorResultRepository struct {
	db dbRunner
}

func NewIndicatorResultRepository(db dbRunner) *IndicatorResultRepository {
	return &IndicatorResultRepository{db: db}
}

func (r *IndicatorResultRepository) Upsert(ctx context.Context, klineID string, indicators map[string]float64) error {
	payload, err := json.Marshal(indicators)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO market_kline_indicator_results (kline_id, indicators, updated_at)
VALUES ($1, $2::jsonb, NOW())
ON CONFLICT (kline_id)
DO UPDATE SET indicators = EXCLUDED.indicators, updated_at = NOW()`, klineID, string(payload))
	return err
}
