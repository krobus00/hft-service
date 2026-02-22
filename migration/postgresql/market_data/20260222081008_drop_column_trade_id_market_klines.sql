-- +goose Up
-- +goose StatementBegin
ALTER TABLE market_klines
DROP COLUMN first_trade_id,
DROP COLUMN last_trade_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE market_klines
ADD COLUMN first_trade_id BIGINT,
ADD COLUMN last_trade_id BIGINT;
-- +goose StatementEnd
