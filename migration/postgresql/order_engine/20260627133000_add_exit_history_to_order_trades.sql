-- +goose Up
-- +goose StatementBegin
ALTER TABLE order_trades
    ADD COLUMN IF NOT EXISTS exit_history_id VARCHAR REFERENCES order_histories(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_order_trades_exit_history_id
    ON order_trades (exit_history_id)
    WHERE exit_history_id IS NOT NULL;

CREATE OR REPLACE FUNCTION refresh_order_trade(pair_id TEXT) RETURNS VOID AS $$
BEGIN
    IF pair_id IS NULL OR pair_id = '' THEN
        RETURN;
    END IF;

    INSERT INTO order_trades (
        entry_order_id, entry_history_id, exit_history_id, strategy_id, exchange, market_type, symbol, side,
        entry_price, exit_price, quantity, realized_pnl, entry_time, exit_time, updated_at
    )
    SELECT
        entry.entry_order_id,
        entry.id,
        closed_order.exit_history_id,
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
        SELECT id AS exit_history_id, COALESCE(avg_fill_price, price) AS exit_price, realized_pnl, created_at AS exit_time
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
        exit_history_id = EXCLUDED.exit_history_id,
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

UPDATE order_trades trade
SET exit_history_id = closed_order.id
FROM LATERAL (
    SELECT id
    FROM order_histories
    WHERE entry_order_id = trade.entry_order_id
      AND trade_condition <> 'ENTRY'
      AND status = 'FILLED'
      AND COALESCE(avg_fill_price, price) IS NOT NULL
    ORDER BY created_at DESC
    LIMIT 1
) closed_order
WHERE trade.exit_time IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_order_trades_exit_history_id;
ALTER TABLE order_trades DROP COLUMN IF EXISTS exit_history_id;
-- +goose StatementEnd
