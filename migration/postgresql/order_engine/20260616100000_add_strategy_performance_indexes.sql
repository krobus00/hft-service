-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_order_histories_strategy_performance
ON order_histories (strategy_id, created_at DESC, entry_order_id)
INCLUDE (symbol, trade_condition, avg_fill_price, filled_quantity, quantity, side)
WHERE entry_order_id <> '' AND avg_fill_price IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_order_histories_strategy_performance;
-- +goose StatementEnd
