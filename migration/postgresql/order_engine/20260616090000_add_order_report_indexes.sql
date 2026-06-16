-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_order_histories_report_created_entry
ON order_histories (created_at DESC, entry_order_id)
INCLUDE (strategy_id, symbol, trade_condition, avg_fill_price, side)
WHERE entry_order_id <> '' AND avg_fill_price IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_order_histories_report_strategy_symbol_time
ON order_histories (strategy_id, symbol, created_at DESC)
WHERE entry_order_id <> '' AND avg_fill_price IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_order_histories_report_strategy_symbol_time;
DROP INDEX IF EXISTS idx_order_histories_report_created_entry;
-- +goose StatementEnd
