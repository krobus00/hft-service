-- +goose Up
-- +goose StatementBegin
ALTER TABLE order_histories
    ADD COLUMN IF NOT EXISTS order_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS exit_type VARCHAR(32) NOT NULL DEFAULT '';

UPDATE order_histories
SET
    order_reason = COALESCE(order_reason, ''),
    exit_type = UPPER(COALESCE(exit_type, ''))
WHERE
    order_reason IS NULL OR exit_type IS NULL OR exit_type <> UPPER(exit_type);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE order_histories
    DROP COLUMN IF EXISTS order_reason,
    DROP COLUMN IF EXISTS exit_type;
-- +goose StatementEnd
