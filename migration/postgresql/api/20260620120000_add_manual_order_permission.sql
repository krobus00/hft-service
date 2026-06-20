-- +goose Up
-- +goose StatementBegin
INSERT INTO permissions (name, description)
VALUES ('order:write', 'Create manual trades and close running positions')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.name = 'order:write'
WHERE r.name = 'admin' ON CONFLICT DO NOTHING;

UPDATE dashboard_pages SET write_permission = 'order:write' WHERE resource_key = 'orders';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE dashboard_pages SET write_permission = '' WHERE resource_key = 'orders';
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name = 'order:write');
DELETE FROM permissions WHERE name = 'order:write';
-- +goose StatementEnd
