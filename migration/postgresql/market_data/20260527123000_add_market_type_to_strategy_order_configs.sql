-- +goose Up
-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    ADD COLUMN IF NOT EXISTS market_type VARCHAR(16);
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE strategy_order_configs
SET market_type = 'spot'
WHERE market_type IS NULL OR length(trim(market_type)) = 0;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    ALTER COLUMN market_type SET DEFAULT 'spot',
    ALTER COLUMN market_type SET NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    DROP CONSTRAINT IF EXISTS uq_strategy_order_configs;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    ADD CONSTRAINT uq_strategy_order_configs
    UNIQUE (strategy, exchange, symbol, market_type, interval);
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_strategy_order_configs_lookup;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_strategy_order_configs_lookup
    ON strategy_order_configs (exchange, symbol, market_type, interval);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    DROP CONSTRAINT IF EXISTS chk_strategy_order_configs_market_type_nonempty;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    ADD CONSTRAINT chk_strategy_order_configs_market_type_nonempty
    CHECK (length(trim(market_type)) > 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    DROP CONSTRAINT IF EXISTS chk_strategy_order_configs_market_type_nonempty;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_strategy_order_configs_lookup;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_strategy_order_configs_lookup
    ON strategy_order_configs (exchange, symbol, interval);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    DROP CONSTRAINT IF EXISTS uq_strategy_order_configs;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    ADD CONSTRAINT uq_strategy_order_configs
    UNIQUE (strategy, exchange, symbol, interval);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    ALTER COLUMN market_type DROP DEFAULT,
    ALTER COLUMN market_type DROP NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_order_configs
    DROP COLUMN IF EXISTS market_type;
-- +goose StatementEnd
