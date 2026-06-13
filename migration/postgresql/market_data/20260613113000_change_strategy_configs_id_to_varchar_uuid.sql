-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE IF EXISTS strategy_configs
    ALTER COLUMN id DROP DEFAULT;

ALTER TABLE IF EXISTS strategy_configs
    ALTER COLUMN id TYPE VARCHAR USING id::VARCHAR;

ALTER TABLE IF EXISTS strategy_configs
    ALTER COLUMN id SET DEFAULT gen_random_uuid()::TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE IF EXISTS strategy_configs
    ALTER COLUMN id DROP DEFAULT;

CREATE SEQUENCE IF NOT EXISTS strategy_configs_id_seq;

ALTER TABLE IF EXISTS strategy_configs
    ALTER COLUMN id TYPE BIGINT USING nextval('strategy_configs_id_seq');

SELECT setval(
    'strategy_configs_id_seq',
    COALESCE((SELECT MAX(id) FROM strategy_configs), 0) + 1,
    false
);

ALTER TABLE IF EXISTS strategy_configs
    ALTER COLUMN id SET DEFAULT nextval('strategy_configs_id_seq'::regclass);

ALTER SEQUENCE strategy_configs_id_seq OWNED BY strategy_configs.id;
-- +goose StatementEnd
