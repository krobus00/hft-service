-- +goose Up
-- +goose StatementBegin
INSERT INTO permissions (name, description)
VALUES
    ('order:write', 'Create, update, and delete order history'),
    ('market:write', 'Create, update, and delete market kline data')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'admin'
  AND p.name IN ('order:write', 'market:write')
ON CONFLICT DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE name IN ('order:write', 'market:write')
);

DELETE FROM permissions WHERE name IN ('order:write', 'market:write');
-- +goose StatementEnd
