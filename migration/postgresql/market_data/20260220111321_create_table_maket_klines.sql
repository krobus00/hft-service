-- +goose Up
-- +goose StatementBegin
------------------------------------------------------------
-- 1️⃣ Parent Table
------------------------------------------------------------

CREATE TABLE IF NOT EXISTS market_klines (
    exchange          VARCHAR(20) NOT NULL,

    event_type        VARCHAR(20) NOT NULL,
    event_time        TIMESTAMPTZ NOT NULL,

    symbol            VARCHAR(20) NOT NULL,
    interval          VARCHAR(10) NOT NULL,

    open_time         TIMESTAMPTZ NOT NULL,
    close_time        TIMESTAMPTZ NOT NULL,

    open_price        NUMERIC(38,18) NOT NULL,
    high_price        NUMERIC(38,18) NOT NULL,
    low_price         NUMERIC(38,18) NOT NULL,
    close_price       NUMERIC(38,18) NOT NULL,

    base_volume       NUMERIC(38,18) NOT NULL,
    quote_volume      NUMERIC(38,18) NOT NULL,
    taker_base_volume NUMERIC(38,18) NOT NULL,
    taker_quote_volume NUMERIC(38,18) NOT NULL,

    trade_count       INTEGER NOT NULL,
    is_closed         BOOLEAN NOT NULL DEFAULT FALSE,

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (exchange, symbol, interval, open_time)

);


------------------------------------------------------------
-- 2️⃣ Global Indexes (auto created per partition)
------------------------------------------------------------

CREATE INDEX IF NOT EXISTS idx_kline_lookup
    ON market_klines (exchange, symbol, interval, open_time DESC);

CREATE INDEX IF NOT EXISTS idx_kline_closed
    ON market_klines (exchange, symbol, interval, open_time DESC)
    WHERE is_closed = TRUE;

CREATE INDEX IF NOT EXISTS idx_kline_time
    ON market_klines (exchange, open_time);


-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query';
-- +goose StatementEnd
