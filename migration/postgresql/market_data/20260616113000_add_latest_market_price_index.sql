-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_market_klines_latest_price
ON market_klines (exchange, market_type, symbol, open_time DESC, updated_at DESC)
INCLUDE (close_price);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_market_klines_latest_price;
-- +goose StatementEnd
