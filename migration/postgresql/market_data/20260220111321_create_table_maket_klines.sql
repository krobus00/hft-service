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

    first_trade_id    BIGINT NOT NULL,
    last_trade_id     BIGINT NOT NULL,

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

    -- IMPORTANT: must include partition keys
    PRIMARY KEY (exchange, symbol, interval, open_time)

) PARTITION BY LIST (symbol);


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


------------------------------------------------------------
-- 3️⃣ Function: Create Symbol Partition
------------------------------------------------------------

CREATE OR REPLACE FUNCTION create_symbol_partition(p_symbol TEXT)
RETURNS VOID AS
$$
DECLARE
    table_name TEXT;
BEGIN
    table_name := format('market_klines_%s', lower(p_symbol));

    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I
         PARTITION OF market_klines
         FOR VALUES IN (%L)
         PARTITION BY RANGE (open_time);',
        table_name,
        p_symbol
    );
END;
$$ LANGUAGE plpgsql;


------------------------------------------------------------
-- 4️⃣ Function: Create Monthly Partition
------------------------------------------------------------

CREATE OR REPLACE FUNCTION create_month_partition(
    p_symbol TEXT,
    p_year   INT,
    p_month  INT
)
RETURNS VOID AS
$$
DECLARE
    parent_table TEXT;
    partition_table TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    parent_table := format('market_klines_%s', lower(p_symbol));

    start_date := make_date(p_year, p_month, 1);
    end_date := (start_date + INTERVAL '1 month')::DATE;

    partition_table := format(
        '%s_%s',
        parent_table,
        to_char(start_date, 'YYYY_MM')
    );

    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I
         PARTITION OF %I
         FOR VALUES FROM (%L) TO (%L);',
        partition_table,
        parent_table,
        start_date,
        end_date
    );
END;
$$ LANGUAGE plpgsql;
------------------------------------------------------------
-- 5️⃣ Example Bootstrap Partitions
------------------------------------------------------------

-- -- Example: create BTCUSDT partition
-- SELECT create_symbol_partition('BTCUSDT');
-- SELECT create_month_partition('BTCUSDT', 2026, 2);

-- -- Example: create SOLUSDT partition
-- SELECT create_symbol_partition('SOLUSDT');
-- SELECT create_month_partition('SOLUSDT', 2026, 2);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query';
-- +goose StatementEnd
