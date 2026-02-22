-- +goose Up
-- +goose StatementBegin
CREATE TABLE order_histories (
    id                  VARCHAR PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             VARCHAR     NOT NULL,

    -- Exchange Info
    exchange            VARCHAR(20)     NOT NULL,
    symbol              VARCHAR(20)     NOT NULL,

    -- Identifiers
    order_id            VARCHAR(64)     NOT NULL,
    client_order_id     VARCHAR(64),

    -- Order Details
    side                VARCHAR(50)        NOT NULL, -- 1=BUY, 2=SELL
    type                VARCHAR(50)        NOT NULL, -- 1=LIMIT,2=MARKET,3=STOP
    price               NUMERIC(20,10),
    quantity            NUMERIC(20,10)  NOT NULL,
    filled_quantity     NUMERIC(20,10)  DEFAULT 0,
    avg_fill_price      NUMERIC(20,10),

    -- Status
    status              VARCHAR(50)        NOT NULL, -- 1=NEW,2=PARTIAL,3=FILLED,4=CANCELED,5=REJECTED

    -- Risk / Margin
    leverage            NUMERIC(10,2),
    fee                 NUMERIC(20,10),
    realized_pnl        NUMERIC(20,10),

    -- Latency Tracking (VERY IMPORTANT IN HFT)
    created_at_exchange TIMESTAMPTZ,
    sent_at             TIMESTAMPTZ,
    acknowledged_at     TIMESTAMPTZ,
    filled_at           TIMESTAMPTZ,

    -- System
    strategy_id         VARCHAR(50),
    error_message       TEXT,

    created_at          TIMESTAMPTZ DEFAULT now() NOT NULL,
    updated_at          TIMESTAMPTZ DEFAULT now() NOT NULL
);

-- lookup by exchange order id
CREATE UNIQUE INDEX idx_order_histories_order_id 
ON order_histories(user_id, exchange, order_id);

-- client order id lookup
CREATE INDEX idx_order_histories_client_order_id
ON order_histories(user_id, client_order_id);

-- time-based queries (most common)
CREATE INDEX idx_order_histories_created_at
ON order_histories(user_id, created_at DESC);

-- strategy analytics
CREATE INDEX idx_order_histories_strategy_time
ON order_histories(user_id, strategy_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE order_histories;
-- +goose StatementEnd
