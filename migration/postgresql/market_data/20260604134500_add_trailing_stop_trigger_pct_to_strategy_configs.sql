-- +goose Up
-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    ADD COLUMN IF NOT EXISTS trailing_stop_trigger_pct NUMERIC(10, 6) NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_trailing_stop_trigger_pct_nonnegative;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    ADD CONSTRAINT chk_strategy_configs_trailing_stop_trigger_pct_nonnegative CHECK (trailing_stop_trigger_pct >= 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_trailing_stop_trigger_pct_nonnegative;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    DROP COLUMN IF EXISTS trailing_stop_trigger_pct;
-- +goose StatementEnd
