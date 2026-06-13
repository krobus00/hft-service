-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE market_klines
ADD COLUMN IF NOT EXISTS id VARCHAR DEFAULT gen_random_uuid();

UPDATE market_klines
SET id = gen_random_uuid()
WHERE id IS NULL;

ALTER TABLE market_klines
ALTER COLUMN id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_market_klines_id ON market_klines (id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_market_klines_id;
ALTER TABLE market_klines DROP COLUMN IF EXISTS id;
-- +goose StatementEnd
