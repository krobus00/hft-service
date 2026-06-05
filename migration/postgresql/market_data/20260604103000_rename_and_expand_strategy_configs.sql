-- +goose Up
-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_order_configs RENAME TO strategy_configs;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER INDEX IF EXISTS idx_strategy_order_configs_lookup RENAME TO idx_strategy_configs_lookup;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT uq_strategy_order_configs TO uq_strategy_configs;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT chk_strategy_order_configs_order_type_nonempty TO chk_strategy_configs_order_type_nonempty;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT chk_strategy_order_configs_order_qty_positive TO chk_strategy_configs_order_qty_positive;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT chk_strategy_order_configs_limit_slippage_nonnegative TO chk_strategy_configs_limit_slippage_nonnegative;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT chk_strategy_order_configs_market_type_nonempty TO chk_strategy_configs_market_type_nonempty;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_configs
    ADD COLUMN IF NOT EXISTS cooldown_bars INTEGER NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS sl_cooldown_bars INTEGER NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS max_consecutive_stop_losses INTEGER NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS sl_pause_bars INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS take_profit_pct NUMERIC(10, 6) NOT NULL DEFAULT 0.25,
    ADD COLUMN IF NOT EXISTS stop_loss_pct NUMERIC(10, 6) NOT NULL DEFAULT 0.15,
    ADD COLUMN IF NOT EXISTS trailing_stop_pct NUMERIC(10, 6) NOT NULL DEFAULT 0.12,
    ADD COLUMN IF NOT EXISTS trailing_stop_trigger_pct NUMERIC(10, 6) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_hold_bars INTEGER NOT NULL DEFAULT 24,
    ADD COLUMN IF NOT EXISTS max_positions INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS enable_intrabar_risk_exit BOOLEAN NOT NULL DEFAULT TRUE;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_configs
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_cooldown_bars_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_sl_cooldown_bars_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_max_consecutive_stop_losses_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_sl_pause_bars_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_take_profit_pct_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_stop_loss_pct_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_trailing_stop_pct_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_trailing_stop_trigger_pct_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_max_hold_bars_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_max_positions_positive;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE strategy_configs
    ADD CONSTRAINT chk_strategy_configs_cooldown_bars_nonnegative CHECK (cooldown_bars >= 0),
    ADD CONSTRAINT chk_strategy_configs_sl_cooldown_bars_nonnegative CHECK (sl_cooldown_bars >= 0),
    ADD CONSTRAINT chk_strategy_configs_max_consecutive_stop_losses_nonnegative CHECK (max_consecutive_stop_losses >= 0),
    ADD CONSTRAINT chk_strategy_configs_sl_pause_bars_nonnegative CHECK (sl_pause_bars >= 0),
    ADD CONSTRAINT chk_strategy_configs_take_profit_pct_nonnegative CHECK (take_profit_pct >= 0),
    ADD CONSTRAINT chk_strategy_configs_stop_loss_pct_nonnegative CHECK (stop_loss_pct >= 0),
    ADD CONSTRAINT chk_strategy_configs_trailing_stop_pct_nonnegative CHECK (trailing_stop_pct >= 0),
    ADD CONSTRAINT chk_strategy_configs_trailing_stop_trigger_pct_nonnegative CHECK (trailing_stop_trigger_pct >= 0),
    ADD CONSTRAINT chk_strategy_configs_max_hold_bars_nonnegative CHECK (max_hold_bars >= 0),
    ADD CONSTRAINT chk_strategy_configs_max_positions_positive CHECK (max_positions > 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_cooldown_bars_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_sl_cooldown_bars_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_max_consecutive_stop_losses_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_sl_pause_bars_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_take_profit_pct_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_stop_loss_pct_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_trailing_stop_pct_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_trailing_stop_trigger_pct_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_max_hold_bars_nonnegative,
    DROP CONSTRAINT IF EXISTS chk_strategy_configs_max_positions_positive;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    DROP COLUMN IF EXISTS cooldown_bars,
    DROP COLUMN IF EXISTS sl_cooldown_bars,
    DROP COLUMN IF EXISTS max_consecutive_stop_losses,
    DROP COLUMN IF EXISTS sl_pause_bars,
    DROP COLUMN IF EXISTS take_profit_pct,
    DROP COLUMN IF EXISTS stop_loss_pct,
    DROP COLUMN IF EXISTS trailing_stop_pct,
    DROP COLUMN IF EXISTS trailing_stop_trigger_pct,
    DROP COLUMN IF EXISTS max_hold_bars,
    DROP COLUMN IF EXISTS max_positions,
    DROP COLUMN IF EXISTS enable_intrabar_risk_exit;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT uq_strategy_configs TO uq_strategy_order_configs;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT chk_strategy_configs_order_type_nonempty TO chk_strategy_order_configs_order_type_nonempty;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT chk_strategy_configs_order_qty_positive TO chk_strategy_order_configs_order_qty_positive;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT chk_strategy_configs_limit_slippage_nonnegative TO chk_strategy_order_configs_limit_slippage_nonnegative;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    RENAME CONSTRAINT chk_strategy_configs_market_type_nonempty TO chk_strategy_order_configs_market_type_nonempty;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER INDEX IF EXISTS idx_strategy_configs_lookup RENAME TO idx_strategy_order_configs_lookup;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs RENAME TO strategy_order_configs;
-- +goose StatementEnd
