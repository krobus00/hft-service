-- +goose Up
-- +goose StatementBegin
SELECT 'noop: partitioning removed';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'noop';
-- +goose StatementEnd
