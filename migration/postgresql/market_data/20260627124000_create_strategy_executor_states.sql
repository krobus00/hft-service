-- +goose Up
CREATE TABLE IF NOT EXISTS strategy_executor_states (
    exchange TEXT NOT NULL,
    market_type TEXT NOT NULL DEFAULT 'spot',
    symbol TEXT NOT NULL,
    interval TEXT NOT NULL,
    values JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (exchange, market_type, symbol, interval)
);

-- +goose Down
DROP TABLE IF EXISTS strategy_executor_states;
