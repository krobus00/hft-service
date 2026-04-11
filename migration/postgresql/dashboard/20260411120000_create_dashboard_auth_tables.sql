-- +goose Up
-- +goose StatementBegin
CREATE TABLE dashboard_users (
    id              VARCHAR PRIMARY KEY DEFAULT gen_random_uuid(),
    username        VARCHAR(100) UNIQUE NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    role            VARCHAR(50) NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dashboard_refresh_tokens (
    id              VARCHAR PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         VARCHAR NOT NULL REFERENCES dashboard_users(id) ON DELETE CASCADE,
    token_hash      VARCHAR(64) NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dashboard_refresh_tokens_user_id
ON dashboard_refresh_tokens(user_id);

CREATE INDEX idx_dashboard_refresh_tokens_expires_at
ON dashboard_refresh_tokens(expires_at);

INSERT INTO dashboard_users (username, password_hash, role)
VALUES ('admin', '$2a$10$mhJOp90/nmINlHXjat0aMOSonijfgyHFuYOVHrgG2CJGnsD4VqfnK', 'admin');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE dashboard_refresh_tokens;
DROP TABLE dashboard_users;
-- +goose StatementEnd
