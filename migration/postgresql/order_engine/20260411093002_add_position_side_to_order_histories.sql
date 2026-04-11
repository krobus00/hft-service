-- +goose Up
-- +goose StatementBegin
ALTER TABLE order_histories
    ADD COLUMN IF NOT EXISTS position_side VARCHAR(10) NOT NULL DEFAULT 'BOTH';

UPDATE order_histories
SET position_side = 'BOTH'
WHERE position_side IS NULL OR btrim(position_side) = '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE order_histories
    DROP COLUMN IF EXISTS position_side;
-- +goose StatementEnd
