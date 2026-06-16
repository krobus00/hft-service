-- +goose Up
-- +goose StatementBegin
INSERT INTO permissions (name, description)
VALUES
    ('order_report:read', 'Read order profit and loss reports')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name = 'order_report:read'
WHERE r.name IN ('admin', 'viewer')
ON CONFLICT DO NOTHING;

INSERT INTO dashboard_pages (resource_key, parent_key, label, description, short_description, icon, path, read_permission, write_permission, sort_order, visible)
VALUES
    ('orderPnL', 'Trading', 'Order PnL', 'Review closed paired trades with per-trade and running profit or loss.', 'Running PnL', 'line-chart', '/dashboard/orderPnL', 'order_report:read', '', 12, TRUE),
    ('dailyReports', 'Trading', 'Daily Reports', 'Daily profit and loss grouped by strategy and symbol.', 'Daily PnL', 'calendar-days', '/dashboard/dailyReports', 'order_report:read', '', 14, TRUE)
ON CONFLICT (resource_key) DO UPDATE SET
    parent_key = EXCLUDED.parent_key,
    label = EXCLUDED.label,
    description = EXCLUDED.description,
    short_description = EXCLUDED.short_description,
    icon = EXCLUDED.icon,
    path = EXCLUDED.path,
    read_permission = EXCLUDED.read_permission,
    write_permission = EXCLUDED.write_permission,
    sort_order = EXCLUDED.sort_order,
    visible = EXCLUDED.visible,
    updated_at = NOW();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM dashboard_pages WHERE resource_key IN ('orderPnL', 'dailyReports');
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE name = 'order_report:read');
DELETE FROM permissions WHERE name = 'order_report:read';
-- +goose StatementEnd
