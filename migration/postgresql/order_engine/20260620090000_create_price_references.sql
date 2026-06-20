-- +goose Up
-- +goose StatementBegin
CREATE TABLE price_references (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    exchange    VARCHAR(20) NOT NULL,
    market_type VARCHAR(20) NOT NULL,
    symbol      VARCHAR(30) NOT NULL,
    price       NUMERIC(38,18) NOT NULL,
    event_time  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (exchange, market_type, symbol)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE price_references;
-- +goose StatementEnd
