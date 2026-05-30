-- +goose Up
-- +goose StatementBegin
ALTER TABLE order_histories
    ADD COLUMN IF NOT EXISTS entry_order_id VARCHAR(64) NOT NULL DEFAULT '';

UPDATE order_histories
SET entry_order_id = COALESCE(NULLIF(entry_order_id, ''), order_id)
WHERE entry_order_id = '' OR entry_order_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_order_histories_entry_order_id
ON order_histories(user_id, entry_order_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_order_histories_entry_order_id;

ALTER TABLE order_histories
    DROP COLUMN IF EXISTS entry_order_id;
-- +goose StatementEnd
