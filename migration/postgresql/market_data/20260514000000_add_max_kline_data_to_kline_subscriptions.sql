-- +goose Up
-- +goose StatementBegin
ALTER TABLE kline_subscriptions
    ADD COLUMN IF NOT EXISTS max_kline_data INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN kline_subscriptions.max_kline_data IS 'Maximum number of kline records to retain per (exchange, market_type, symbol, interval). 0 means unlimited.';
-- +goose StatementEnd

-- +goose StatementBegin
-- Trigger function: runs after INSERT or UPDATE on market_klines.
-- On closed candles (is_closed = true), looks up the subscription's max_kline_data and
-- deletes the oldest rows beyond the limit using a single boundary-seek on idx_kline_lookup.
-- Uses OFFSET to find the (N+1)th row, then deletes everything with open_time < that value.
-- If total rows <= max_kline_data, the subquery returns NULL and nothing is deleted.
CREATE OR REPLACE FUNCTION fn_prune_market_klines()
RETURNS TRIGGER AS $$
DECLARE
    v_max INT;
BEGIN
    IF NOT NEW.is_closed THEN
        RETURN NEW;
    END IF;

    SELECT max_kline_data INTO v_max
    FROM kline_subscriptions
    WHERE exchange    = NEW.exchange
      AND market_type = NEW.market_type
      AND symbol      = NEW.symbol
      AND interval    = NEW.interval
    LIMIT 1;

    IF v_max IS NULL OR v_max <= 0 THEN
        RETURN NEW;
    END IF;

    DELETE FROM market_klines
    WHERE exchange    = NEW.exchange
      AND market_type = NEW.market_type
      AND symbol      = NEW.symbol
      AND interval    = NEW.interval
      AND open_time < (
          SELECT open_time
          FROM market_klines
          WHERE exchange    = NEW.exchange
            AND market_type = NEW.market_type
            AND symbol      = NEW.symbol
            AND interval    = NEW.interval
          ORDER BY open_time DESC
          OFFSET v_max
          LIMIT 1
      );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE TRIGGER trg_prune_market_klines
AFTER INSERT OR UPDATE ON market_klines
FOR EACH ROW
EXECUTE FUNCTION fn_prune_market_klines();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_prune_market_klines ON market_klines;
-- +goose StatementEnd

-- +goose StatementBegin
DROP FUNCTION IF EXISTS fn_prune_market_klines();
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE kline_subscriptions
    DROP COLUMN IF EXISTS max_kline_data;
-- +goose StatementEnd
