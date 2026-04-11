-- +goose Up
-- +goose StatementBegin
ALTER TABLE order_histories
    ADD COLUMN IF NOT EXISTS market_type VARCHAR(20) NOT NULL DEFAULT 'spot';

UPDATE order_histories
SET market_type = 'spot'
WHERE market_type IS NULL OR btrim(market_type) = '';

DROP INDEX IF EXISTS idx_order_histories_order_id;

CREATE UNIQUE INDEX idx_order_histories_order_id
ON order_histories(user_id, exchange, market_type, order_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_order_histories_order_id;

CREATE UNIQUE INDEX idx_order_histories_order_id
ON order_histories(user_id, exchange, order_id);

ALTER TABLE order_histories
    DROP COLUMN IF EXISTS market_type;
-- +goose StatementEnd
