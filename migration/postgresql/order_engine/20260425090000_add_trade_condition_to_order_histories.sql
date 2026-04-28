-- +goose Up
-- +goose StatementBegin
ALTER TABLE order_histories
    ADD COLUMN IF NOT EXISTS trade_condition VARCHAR(32) NOT NULL DEFAULT 'UNKNOWN';

UPDATE order_histories
SET trade_condition = 'UNKNOWN'
WHERE trade_condition IS NULL OR btrim(trade_condition) = '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE order_histories
    DROP COLUMN IF EXISTS trade_condition;
-- +goose StatementEnd
