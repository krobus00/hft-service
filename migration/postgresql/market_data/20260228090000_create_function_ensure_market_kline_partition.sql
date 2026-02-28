-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_market_kline_partition(
    p_symbol    TEXT,
    p_open_time TIMESTAMPTZ
)
RETURNS VOID AS
$$
DECLARE
    safe_symbol TEXT;
BEGIN
    safe_symbol := upper(p_symbol);

    PERFORM pg_advisory_xact_lock(hashtext('market_klines_partition_' || safe_symbol));

    PERFORM create_symbol_partition(safe_symbol);
    PERFORM create_month_partition(
        safe_symbol,
        EXTRACT(YEAR FROM p_open_time AT TIME ZONE 'UTC')::INT,
        EXTRACT(MONTH FROM p_open_time AT TIME ZONE 'UTC')::INT
    );
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS market_klines_default
PARTITION OF market_klines DEFAULT
PARTITION BY RANGE (open_time);

CREATE TABLE IF NOT EXISTS market_klines_default_fallback
PARTITION OF market_klines_default DEFAULT;

CREATE OR REPLACE FUNCTION reroute_market_kline_from_default()
RETURNS TRIGGER AS
$$
BEGIN
    IF pg_trigger_depth() > 1 THEN
        RAISE EXCEPTION 'partition reroute recursion detected for symbol %, open_time %', NEW.symbol, NEW.open_time;
    END IF;

    PERFORM ensure_market_kline_partition(NEW.symbol, NEW.open_time);

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
    VALUES (
        NEW.exchange,
        NEW.event_type,
        NEW.event_time,
        NEW.symbol,
        NEW.interval,
        NEW.open_time,
        NEW.close_time,
        NEW.open_price,
        NEW.high_price,
        NEW.low_price,
        NEW.close_price,
        NEW.base_volume,
        NEW.quote_volume,
        NEW.taker_base_volume,
        NEW.taker_quote_volume,
        NEW.trade_count,
        NEW.is_closed,
        NEW.created_at,
        NEW.updated_at
    )
    ON CONFLICT (exchange, symbol, interval, open_time)
    DO UPDATE SET
        event_type = EXCLUDED.event_type,
        event_time = EXCLUDED.event_time,
        close_time = EXCLUDED.close_time,
        open_price = EXCLUDED.open_price,
        high_price = EXCLUDED.high_price,
        low_price = EXCLUDED.low_price,
        close_price = EXCLUDED.close_price,
        base_volume = EXCLUDED.base_volume,
        quote_volume = EXCLUDED.quote_volume,
        taker_base_volume = EXCLUDED.taker_base_volume,
        taker_quote_volume = EXCLUDED.taker_quote_volume,
        trade_count = EXCLUDED.trade_count,
        is_closed = EXCLUDED.is_closed,
        updated_at = EXCLUDED.updated_at;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_reroute_market_klines_default ON market_klines_default_fallback;

CREATE TRIGGER trg_reroute_market_klines_default
BEFORE INSERT ON market_klines_default_fallback
FOR EACH ROW
EXECUTE FUNCTION reroute_market_kline_from_default();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_reroute_market_klines_default ON market_klines_default_fallback;
DROP FUNCTION IF EXISTS reroute_market_kline_from_default();
DROP TABLE IF EXISTS market_klines_default_fallback;
DROP TABLE IF EXISTS market_klines_default;
DROP FUNCTION IF EXISTS ensure_market_kline_partition(TEXT, TIMESTAMPTZ);
-- +goose StatementEnd
