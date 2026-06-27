-- +goose Up
DROP INDEX IF EXISTS idx_strategy_configs_monitor_url;
ALTER TABLE strategy_configs
    DROP COLUMN IF EXISTS monitor_url;

-- +goose Down
ALTER TABLE strategy_configs
    ADD COLUMN IF NOT EXISTS monitor_url TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_strategy_configs_monitor_url
    ON strategy_configs (monitor_url)
    WHERE enabled AND btrim(monitor_url) <> '';
