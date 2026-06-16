-- +goose Up
-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS idx_strategy_configs_enabled_lookup
    ON strategy_configs (enabled, exchange, market_type, symbol, interval);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_strategy_configs_enabled_lookup;

ALTER TABLE IF EXISTS strategy_configs
    DROP COLUMN IF EXISTS enabled;
-- +goose StatementEnd
