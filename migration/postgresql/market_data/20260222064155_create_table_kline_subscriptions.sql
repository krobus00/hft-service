-- +goose Up
-- +goose StatementBegin
CREATE TABLE kline_subscriptions (
    id              VARCHAR PRIMARY KEY DEFAULT gen_random_uuid(),
    exchange        VARCHAR(20) NOT NULL,
    symbol          VARCHAR(30) NOT NULL,
    interval        VARCHAR(10) NOT NULL,
    payload         JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (exchange, symbol, interval)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE kline_subscriptions;
-- +goose StatementEnd
