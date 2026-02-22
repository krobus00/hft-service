-- +goose Up
-- +goose StatementBegin
ALTER TABLE order_histories
ADD COLUMN request_id VARCHAR(255);

CREATE INDEX idx_order_histories_request_id ON order_histories(request_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_order_histories_request_id;

ALTER TABLE order_histories
DROP COLUMN request_id;
-- +goose StatementEnd
