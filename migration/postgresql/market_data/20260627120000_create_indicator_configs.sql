-- +goose Up
CREATE TABLE IF NOT EXISTS indicator_configs (
    id VARCHAR PRIMARY KEY DEFAULT gen_random_uuid(),
    exchange TEXT NOT NULL,
    market_type TEXT NOT NULL DEFAULT 'spot',
    symbol TEXT NOT NULL,
    interval TEXT NOT NULL,
    indicator TEXT NOT NULL,
    output_name TEXT NOT NULL,
    params JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_indicator_configs_exchange_nonempty CHECK (btrim(exchange) <> ''),
    CONSTRAINT chk_indicator_configs_market_type_nonempty CHECK (btrim(market_type) <> ''),
    CONSTRAINT chk_indicator_configs_symbol_nonempty CHECK (btrim(symbol) <> ''),
    CONSTRAINT chk_indicator_configs_interval_nonempty CHECK (btrim(interval) <> ''),
    CONSTRAINT chk_indicator_configs_indicator_nonempty CHECK (btrim(indicator) <> ''),
    CONSTRAINT chk_indicator_configs_output_name_nonempty CHECK (btrim(output_name) <> ''),
    CONSTRAINT uq_indicator_configs_output UNIQUE (exchange, market_type, symbol, interval, output_name)
);

CREATE INDEX IF NOT EXISTS idx_indicator_configs_lookup
    ON indicator_configs (enabled, exchange, market_type, symbol, interval);

INSERT INTO indicator_configs (exchange, market_type, symbol, interval, indicator, output_name, params)
SELECT sub.exchange, sub.market_type, sub.symbol, sub.interval, cfg.indicator, cfg.output_name, cfg.params::jsonb
FROM (
    SELECT DISTINCT exchange, market_type, symbol, interval
    FROM kline_subscriptions
) sub
CROSS JOIN (
    VALUES
        ('ema', 'ema_21', '{"period":21}'),
        ('ema', 'ema_50', '{"period":50}'),
        ('ema', 'ema_55', '{"period":55}'),
        ('ema', 'ema_200', '{"period":200}'),
        ('vwap', 'vwap_100', '{"period":100}'),
        ('vwap', 'vwap_120', '{"period":120}'),
        ('vwap', 'vwap_200', '{"period":200}'),
        ('macd', 'macd_12_26_9', '{"fast":12,"slow":26,"signal":9}'),
        ('atr', 'atr_10', '{"period":10}'),
        ('atr', 'atr_14', '{"period":14}'),
        ('rsi', 'rsi_14', '{"period":14}'),
        ('bollinger_bands', 'bb_20_2', '{"period":20}'),
        ('stochastic', 'stoch_14_3', '{"period":14,"signal":3}'),
        ('volume_mean', 'volume_mean_20', '{"period":20}')
) AS cfg(indicator, output_name, params)
ON CONFLICT (exchange, market_type, symbol, interval, output_name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS indicator_configs;
