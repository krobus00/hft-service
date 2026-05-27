-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS strategy_order_configs (
    id BIGSERIAL PRIMARY KEY,
    strategy VARCHAR(128) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    symbol VARCHAR(64) NOT NULL,
    interval VARCHAR(16) NOT NULL,
    need_notification BOOLEAN NOT NULL DEFAULT TRUE,
    is_paper_trading BOOLEAN NOT NULL DEFAULT TRUE,
    order_type VARCHAR(32) NOT NULL,
    order_qty NUMERIC(20, 8) NOT NULL,
    limit_slippage_pct NUMERIC(10, 6) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_strategy_order_configs UNIQUE (strategy, exchange, symbol, interval),
    CONSTRAINT chk_strategy_order_configs_order_type_nonempty CHECK (length(trim(order_type)) > 0),
    CONSTRAINT chk_strategy_order_configs_order_qty_positive CHECK (order_qty > 0),
    CONSTRAINT chk_strategy_order_configs_limit_slippage_nonnegative CHECK (limit_slippage_pct >= 0)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_strategy_order_configs_lookup
    ON strategy_order_configs (exchange, symbol, interval);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_strategy_order_configs_lookup;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS strategy_order_configs;
-- +goose StatementEnd
