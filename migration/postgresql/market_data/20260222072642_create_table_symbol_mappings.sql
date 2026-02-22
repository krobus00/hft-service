-- +goose Up
-- +goose StatementBegin
CREATE TABLE symbol_mappings (
    id              VARCHAR PRIMARY KEY DEFAULT gen_random_uuid(),
    exchange        VARCHAR(20) NOT NULL,
    symbol          VARCHAR(30) NOT NULL,
    kline_symbol    VARCHAR(30) NOT NULL,
    order_symbol    VARCHAR(30) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (exchange, symbol)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE symbol_mappings;
-- +goose StatementEnd
