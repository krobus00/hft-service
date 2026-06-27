-- +goose Up
CREATE TABLE IF NOT EXISTS market_kline_indicator_results (
    kline_id VARCHAR PRIMARY KEY REFERENCES market_klines(id) ON DELETE CASCADE,
    indicators JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_market_kline_indicator_results_indicators
    ON market_kline_indicator_results USING GIN (indicators);

-- +goose Down
DROP TABLE IF EXISTS market_kline_indicator_results;
