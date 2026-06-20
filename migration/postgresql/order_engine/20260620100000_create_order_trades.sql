-- +goose Up
-- +goose StatementBegin
CREATE TABLE order_trades (
    entry_order_id  VARCHAR(64) PRIMARY KEY,
    entry_history_id VARCHAR NOT NULL UNIQUE REFERENCES order_histories(id) ON DELETE CASCADE,
    strategy_id     VARCHAR(50) NOT NULL DEFAULT '',
    exchange        VARCHAR(20) NOT NULL,
    market_type     VARCHAR(20) NOT NULL,
    symbol          VARCHAR(30) NOT NULL,
    side            VARCHAR(50) NOT NULL,
    entry_price     NUMERIC(38,18),
    exit_price      NUMERIC(38,18),
    quantity        NUMERIC(38,18) NOT NULL,
    realized_pnl    NUMERIC(38,18),
    profit          NUMERIC(38,18) GENERATED ALWAYS AS (
        COALESCE(
            realized_pnl,
            CASE
                WHEN side IN ('LONG', 'BUY') THEN (exit_price - entry_price) * quantity
                WHEN side IN ('SHORT', 'SELL') THEN (entry_price - exit_price) * quantity
            END
        )
    ) STORED,
    entry_time      TIMESTAMPTZ NOT NULL,
    exit_time       TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_order_trades_exit_time ON order_trades (exit_time DESC) WHERE exit_time IS NOT NULL;
CREATE INDEX idx_order_trades_strategy_exit ON order_trades (strategy_id, exit_time DESC) WHERE exit_time IS NOT NULL;
CREATE INDEX idx_order_trades_symbol_exit ON order_trades (symbol, exit_time DESC) WHERE exit_time IS NOT NULL;

DROP INDEX IF EXISTS idx_order_histories_report_created_entry;
DROP INDEX IF EXISTS idx_order_histories_report_strategy_symbol_time;
DROP INDEX IF EXISTS idx_order_histories_strategy_performance;

CREATE FUNCTION refresh_order_trade(pair_id TEXT) RETURNS VOID AS $$
BEGIN
    IF pair_id IS NULL OR pair_id = '' THEN
        RETURN;
    END IF;

    INSERT INTO order_trades (
        entry_order_id, entry_history_id, strategy_id, exchange, market_type, symbol, side,
        entry_price, exit_price, quantity, realized_pnl, entry_time, exit_time, updated_at
    )
    SELECT
        entry.entry_order_id,
        entry.id,
        COALESCE(entry.strategy_id, ''),
        entry.exchange,
        entry.market_type,
        entry.symbol,
        entry.side,
        COALESCE(entry.avg_fill_price, entry.price),
        closed_order.exit_price,
        COALESCE(NULLIF(entry.filled_quantity, 0), entry.quantity),
        closed_order.realized_pnl,
        entry.created_at,
        closed_order.exit_time,
        now()
    FROM LATERAL (
        SELECT *
        FROM order_histories
        WHERE entry_order_id = pair_id AND trade_condition = 'ENTRY'
        ORDER BY created_at DESC
        LIMIT 1
    ) entry
    LEFT JOIN LATERAL (
        SELECT COALESCE(avg_fill_price, price) AS exit_price, realized_pnl, created_at AS exit_time
        FROM order_histories
        WHERE entry_order_id = pair_id
          AND trade_condition <> 'ENTRY'
          AND status = 'FILLED'
          AND COALESCE(avg_fill_price, price) IS NOT NULL
        ORDER BY created_at DESC
        LIMIT 1
    ) closed_order ON true
    ON CONFLICT (entry_order_id) DO UPDATE SET
        entry_history_id = EXCLUDED.entry_history_id,
        strategy_id = EXCLUDED.strategy_id,
        exchange = EXCLUDED.exchange,
        market_type = EXCLUDED.market_type,
        symbol = EXCLUDED.symbol,
        side = EXCLUDED.side,
        entry_price = EXCLUDED.entry_price,
        exit_price = EXCLUDED.exit_price,
        quantity = EXCLUDED.quantity,
        realized_pnl = EXCLUDED.realized_pnl,
        entry_time = EXCLUDED.entry_time,
        exit_time = EXCLUDED.exit_time,
        updated_at = EXCLUDED.updated_at;

    IF NOT FOUND THEN
        DELETE FROM order_trades WHERE entry_order_id = pair_id;
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION sync_order_trade() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM refresh_order_trade(OLD.entry_order_id);
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.entry_order_id IS DISTINCT FROM NEW.entry_order_id THEN
        PERFORM refresh_order_trade(OLD.entry_order_id);
    END IF;
    PERFORM refresh_order_trade(NEW.entry_order_id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_order_histories_sync_trade
AFTER INSERT OR UPDATE OR DELETE ON order_histories
FOR EACH ROW EXECUTE FUNCTION sync_order_trade();

INSERT INTO order_trades (
    entry_order_id, entry_history_id, strategy_id, exchange, market_type, symbol, side,
    entry_price, exit_price, quantity, realized_pnl, entry_time, exit_time
)
SELECT
    entry.entry_order_id,
    entry.id,
    COALESCE(entry.strategy_id, ''),
    entry.exchange,
    entry.market_type,
    entry.symbol,
    entry.side,
    COALESCE(entry.avg_fill_price, entry.price),
    closed_order.exit_price,
    COALESCE(NULLIF(entry.filled_quantity, 0), entry.quantity),
    closed_order.realized_pnl,
    entry.created_at,
    closed_order.exit_time
FROM (
    SELECT DISTINCT ON (entry_order_id) *
    FROM order_histories
    WHERE entry_order_id <> '' AND trade_condition = 'ENTRY'
    ORDER BY entry_order_id, created_at DESC
) entry
LEFT JOIN LATERAL (
    SELECT COALESCE(avg_fill_price, price) AS exit_price, realized_pnl, created_at AS exit_time
    FROM order_histories
    WHERE entry_order_id = entry.entry_order_id
      AND trade_condition <> 'ENTRY'
      AND status = 'FILLED'
      AND COALESCE(avg_fill_price, price) IS NOT NULL
    ORDER BY created_at DESC
    LIMIT 1
) closed_order ON true;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_order_histories_sync_trade ON order_histories;
DROP FUNCTION IF EXISTS sync_order_trade();
DROP FUNCTION IF EXISTS refresh_order_trade(TEXT);
DROP TABLE IF EXISTS order_trades;

CREATE INDEX IF NOT EXISTS idx_order_histories_report_created_entry
ON order_histories (created_at DESC, entry_order_id)
INCLUDE (strategy_id, symbol, trade_condition, avg_fill_price, side)
WHERE entry_order_id <> '' AND avg_fill_price IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_order_histories_report_strategy_symbol_time
ON order_histories (strategy_id, symbol, created_at DESC)
WHERE entry_order_id <> '' AND avg_fill_price IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_order_histories_strategy_performance
ON order_histories (strategy_id, created_at DESC, entry_order_id)
INCLUDE (symbol, trade_condition, avg_fill_price, filled_quantity, quantity, side)
WHERE entry_order_id <> '' AND avg_fill_price IS NOT NULL;
-- +goose StatementEnd
