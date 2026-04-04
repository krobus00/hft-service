-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF to_regclass('market_klines_default_fallback') IS NOT NULL THEN
        EXECUTE 'DROP TRIGGER IF EXISTS trg_reroute_market_klines_default ON market_klines_default_fallback';
    END IF;

    IF to_regclass('symbol_mappings') IS NOT NULL THEN
        EXECUTE 'DROP TRIGGER IF EXISTS trg_symbol_mappings_ensure_market_kline_partition ON symbol_mappings';
    END IF;

    DROP FUNCTION IF EXISTS reroute_market_kline_from_default();
    DROP FUNCTION IF EXISTS ensure_market_kline_partition(TEXT, TIMESTAMPTZ);
    DROP FUNCTION IF EXISTS ensure_market_kline_partition_from_symbol_mapping();
    DROP FUNCTION IF EXISTS ensure_market_kline_partitions_for_all_symbols(TIMESTAMPTZ);
    DROP FUNCTION IF EXISTS create_symbol_partition(TEXT);
    DROP FUNCTION IF EXISTS create_month_partition(TEXT, INT, INT);

    DROP TABLE IF EXISTS market_klines_default_fallback CASCADE;
    DROP TABLE IF EXISTS market_klines_default CASCADE;

    IF EXISTS (
        SELECT 1
        FROM pg_partitioned_table pt
        JOIN pg_class c ON c.oid = pt.partrelid
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = 'market_klines'
          AND n.nspname = current_schema()
    ) THEN
        ALTER TABLE market_klines RENAME TO market_klines_partitioned_legacy;

        CREATE TABLE market_klines (
            exchange           VARCHAR(20) NOT NULL,
            event_type         VARCHAR(20) NOT NULL,
            event_time         TIMESTAMPTZ NOT NULL,
            symbol             VARCHAR(20) NOT NULL,
            interval           VARCHAR(10) NOT NULL,
            open_time          TIMESTAMPTZ NOT NULL,
            close_time         TIMESTAMPTZ NOT NULL,
            open_price         NUMERIC(38,18) NOT NULL,
            high_price         NUMERIC(38,18) NOT NULL,
            low_price          NUMERIC(38,18) NOT NULL,
            close_price        NUMERIC(38,18) NOT NULL,
            base_volume        NUMERIC(38,18) NOT NULL,
            quote_volume       NUMERIC(38,18) NOT NULL,
            taker_base_volume  NUMERIC(38,18) NOT NULL,
            taker_quote_volume NUMERIC(38,18) NOT NULL,
            trade_count        INTEGER NOT NULL,
            is_closed          BOOLEAN NOT NULL DEFAULT FALSE,
            created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
            PRIMARY KEY (exchange, symbol, interval, open_time)
        );

        INSERT INTO market_klines (
            exchange,
            event_type,
            event_time,
            symbol,
            interval,
            open_time,
            close_time,
            open_price,
            high_price,
            low_price,
            close_price,
            base_volume,
            quote_volume,
            taker_base_volume,
            taker_quote_volume,
            trade_count,
            is_closed,
            created_at,
            updated_at
        )
        SELECT
            exchange,
            event_type,
            event_time,
            symbol,
            interval,
            open_time,
            close_time,
            open_price,
            high_price,
            low_price,
            close_price,
            base_volume,
            quote_volume,
            taker_base_volume,
            taker_quote_volume,
            trade_count,
            is_closed,
            created_at,
            updated_at
        FROM market_klines_partitioned_legacy;

        CREATE INDEX IF NOT EXISTS idx_kline_lookup
            ON market_klines (exchange, symbol, interval, open_time DESC);

        CREATE INDEX IF NOT EXISTS idx_kline_closed
            ON market_klines (exchange, symbol, interval, open_time DESC)
            WHERE is_closed = TRUE;

        CREATE INDEX IF NOT EXISTS idx_kline_time
            ON market_klines (exchange, open_time);

        DROP TABLE market_klines_partitioned_legacy CASCADE;
    END IF;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'noop';
-- +goose StatementEnd
