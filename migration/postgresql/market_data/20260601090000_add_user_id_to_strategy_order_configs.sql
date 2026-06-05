-- +goose Up
-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    ADD COLUMN IF NOT EXISTS user_id VARCHAR(128);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    DROP COLUMN IF EXISTS user_id;
-- +goose StatementEnd
