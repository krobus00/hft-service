-- +goose Up
-- +goose StatementBegin
ALTER TABLE market_klines
DROP COLUMN IF EXISTS first_trade_id,
DROP COLUMN IF EXISTS last_trade_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE market_klines
ADD COLUMN IF NOT EXISTS first_trade_id BIGINT,
ADD COLUMN IF NOT EXISTS last_trade_id BIGINT;
-- +goose StatementEnd
