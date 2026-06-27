-- +goose Up
CREATE TABLE IF NOT EXISTS strategy_rules (
    id VARCHAR PRIMARY KEY DEFAULT gen_random_uuid(),
    strategy_config_id VARCHAR NOT NULL REFERENCES strategy_configs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    side TEXT NOT NULL,
    trade_condition TEXT NOT NULL DEFAULT 'ENTRY',
    exit_type TEXT NOT NULL DEFAULT '',
    order_reason TEXT NOT NULL DEFAULT '',
    conditions JSONB NOT NULL DEFAULT '{"all":[]}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_strategy_rules_name_nonempty CHECK (btrim(name) <> ''),
    CONSTRAINT chk_strategy_rules_side_nonempty CHECK (btrim(side) <> ''),
    CONSTRAINT chk_strategy_rules_trade_condition_nonempty CHECK (btrim(trade_condition) <> '')
);

CREATE INDEX IF NOT EXISTS idx_strategy_rules_strategy_config_enabled
    ON strategy_rules (strategy_config_id, enabled);

-- +goose Down
DROP TABLE IF EXISTS strategy_rules;
