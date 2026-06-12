-- +goose Up
-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    ADD COLUMN IF NOT EXISTS position_side VARCHAR(20) NOT NULL DEFAULT 'BOTH',
    ADD COLUMN IF NOT EXISTS source VARCHAR(100) NOT NULL DEFAULT 'python-strategy';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_position_side_nonempty,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_source_nonempty;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    ADD CONSTRAINT chk_strategy_configs_position_side_nonempty CHECK (btrim(position_side) <> ''),
    ADD CONSTRAINT chk_strategy_configs_source_nonempty CHECK (btrim(source) <> '');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_position_side_nonempty,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_source_nonempty;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    DROP COLUMN IF EXISTS position_side,
    DROP COLUMN IF EXISTS source;
-- +goose StatementEnd
