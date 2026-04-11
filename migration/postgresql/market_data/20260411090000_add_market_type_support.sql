-- +goose Up
-- +goose StatementBegin
ALTER TABLE symbol_mappings
    ADD COLUMN IF NOT EXISTS market_type VARCHAR(20) NOT NULL DEFAULT 'spot';

UPDATE symbol_mappings
SET market_type = 'spot'
WHERE market_type IS NULL OR btrim(market_type) = '';

ALTER TABLE symbol_mappings
    DROP CONSTRAINT IF EXISTS symbol_mappings_exchange_symbol_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_symbol_mappings_exchange_market_type_symbol
    ON symbol_mappings (exchange, market_type, symbol);

ALTER TABLE kline_subscriptions
    ADD COLUMN IF NOT EXISTS market_type VARCHAR(20) NOT NULL DEFAULT 'spot';

UPDATE kline_subscriptions
SET market_type = 'spot'
WHERE market_type IS NULL OR btrim(market_type) = '';

ALTER TABLE kline_subscriptions
    DROP CONSTRAINT IF EXISTS kline_subscriptions_exchange_symbol_interval_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_kline_subscriptions_exchange_market_type_symbol_interval
    ON kline_subscriptions (exchange, market_type, symbol, interval);

ALTER TABLE market_klines
    ADD COLUMN IF NOT EXISTS market_type VARCHAR(20) NOT NULL DEFAULT 'spot';

ALTER TABLE market_klines
    DROP CONSTRAINT IF EXISTS market_klines_pkey;

ALTER TABLE market_klines
    ADD CONSTRAINT market_klines_pkey PRIMARY KEY (exchange, market_type, symbol, interval, open_time);

DROP INDEX IF EXISTS idx_kline_lookup;
DROP INDEX IF EXISTS idx_kline_closed;
DROP INDEX IF EXISTS idx_kline_time;

CREATE INDEX IF NOT EXISTS idx_kline_lookup
    ON market_klines (exchange, market_type, symbol, interval, open_time DESC);

CREATE INDEX IF NOT EXISTS idx_kline_closed
    ON market_klines (exchange, market_type, symbol, interval, open_time DESC)
    WHERE is_closed = TRUE;

CREATE INDEX IF NOT EXISTS idx_kline_time
    ON market_klines (exchange, market_type, open_time);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_kline_lookup;
DROP INDEX IF EXISTS idx_kline_closed;
DROP INDEX IF EXISTS idx_kline_time;

ALTER TABLE market_klines
    DROP CONSTRAINT IF EXISTS market_klines_pkey;

ALTER TABLE market_klines
    ADD CONSTRAINT market_klines_pkey PRIMARY KEY (exchange, symbol, interval, open_time);

CREATE INDEX IF NOT EXISTS idx_kline_lookup
    ON market_klines (exchange, symbol, interval, open_time DESC);

CREATE INDEX IF NOT EXISTS idx_kline_closed
    ON market_klines (exchange, symbol, interval, open_time DESC)
    WHERE is_closed = TRUE;

CREATE INDEX IF NOT EXISTS idx_kline_time
    ON market_klines (exchange, open_time);

ALTER TABLE market_klines
    DROP COLUMN IF EXISTS market_type;

DROP INDEX IF EXISTS idx_kline_subscriptions_exchange_market_type_symbol_interval;
ALTER TABLE kline_subscriptions
    ADD CONSTRAINT kline_subscriptions_exchange_symbol_interval_key UNIQUE (exchange, symbol, interval);
ALTER TABLE kline_subscriptions
    DROP COLUMN IF EXISTS market_type;

DROP INDEX IF EXISTS idx_symbol_mappings_exchange_market_type_symbol;
ALTER TABLE symbol_mappings
    ADD CONSTRAINT symbol_mappings_exchange_symbol_key UNIQUE (exchange, symbol);
ALTER TABLE symbol_mappings
    DROP COLUMN IF EXISTS market_type;
-- +goose StatementEnd
