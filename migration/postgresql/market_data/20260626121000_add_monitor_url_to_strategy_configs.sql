-- +goose Up
-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    ADD COLUMN IF NOT EXISTS monitor_url TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_strategy_configs_monitor_url
    ON strategy_configs (monitor_url)
    WHERE enabled AND btrim(monitor_url) <> '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_strategy_configs_monitor_url;

ALTER TABLE IF EXISTS strategy_configs
    DROP COLUMN IF EXISTS monitor_url;
-- +goose StatementEnd
