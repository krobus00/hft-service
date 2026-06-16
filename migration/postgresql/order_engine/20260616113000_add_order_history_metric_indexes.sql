-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_order_histories_metrics_pair
ON order_histories (entry_order_id, created_at DESC)
INCLUDE (trade_condition, avg_fill_price, price, realized_pnl)
WHERE entry_order_id <> '' AND trade_condition <> 'ENTRY';

CREATE INDEX IF NOT EXISTS idx_order_histories_metrics_entries_time
ON order_histories (created_at DESC)
INCLUDE (id, strategy_id, exchange, market_type, symbol, side, type, status, entry_order_id, avg_fill_price, price, filled_quantity, quantity)
WHERE trade_condition = 'ENTRY';

CREATE INDEX IF NOT EXISTS idx_order_histories_metrics_entries_strategy_symbol_time
ON order_histories (strategy_id, exchange, market_type, symbol, created_at DESC)
INCLUDE (id, side, type, status, entry_order_id, avg_fill_price, price, filled_quantity, quantity)
WHERE trade_condition = 'ENTRY';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_order_histories_metrics_entries_strategy_symbol_time;
DROP INDEX IF EXISTS idx_order_histories_metrics_entries_time;
DROP INDEX IF EXISTS idx_order_histories_metrics_pair;
-- +goose StatementEnd
