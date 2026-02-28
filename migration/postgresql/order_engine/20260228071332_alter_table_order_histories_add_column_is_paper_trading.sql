-- +goose Up
-- +goose StatementBegin
ALTER TABLE order_histories
ADD COLUMN is_paper_trading BOOLEAN NOT NULL DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE order_histories
DROP COLUMN is_paper_trading;
-- +goose StatementEnd
