-- +goose Up
-- +goose StatementBegin
INSERT INTO settings (key, value)
VALUES ('dashboard.show_setup_link', 'true'::jsonb)
ON CONFLICT (key) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM settings WHERE key = 'dashboard.show_setup_link';
-- +goose StatementEnd
