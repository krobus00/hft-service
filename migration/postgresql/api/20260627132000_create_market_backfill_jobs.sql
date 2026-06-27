-- +goose Up
CREATE TABLE IF NOT EXISTS market_backfill_jobs (
    id VARCHAR PRIMARY KEY,
    status TEXT NOT NULL,
    exchange TEXT NOT NULL,
    market_type TEXT NOT NULL,
    symbol TEXT NOT NULL,
    interval TEXT NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    inserted_count BIGINT NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_market_backfill_jobs_updated_at
    ON market_backfill_jobs (updated_at DESC);

-- +goose Down
DROP TABLE IF EXISTS market_backfill_jobs;
