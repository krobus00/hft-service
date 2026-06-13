-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS dashboard_pages (
    id VARCHAR PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_key VARCHAR(100) NOT NULL UNIQUE,
    parent_key VARCHAR(100) NOT NULL DEFAULT '',
    label VARCHAR(150) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    short_description VARCHAR(255) NOT NULL DEFAULT '',
    icon VARCHAR(100) NOT NULL DEFAULT '',
    path VARCHAR(150) NOT NULL,
    read_permission VARCHAR(150) NOT NULL,
    write_permission VARCHAR(150) NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 100,
    visible BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO permissions (name, description)
VALUES
    ('dashboard_page:read', 'Read dashboard page configuration'),
    ('dashboard_page:write', 'Manage dashboard page configuration'),
    ('permission:read', 'Read API permissions'),
    ('permission:write', 'Manage API permissions')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name IN ('dashboard_page:read', 'dashboard_page:write')
WHERE r.name = 'admin'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name IN ('permission:read', 'permission:write')
WHERE r.name = 'admin'
ON CONFLICT DO NOTHING;

INSERT INTO dashboard_pages (resource_key, parent_key, label, description, short_description, icon, path, read_permission, write_permission, sort_order, visible)
VALUES
    ('orders', 'Trading', 'Orders', 'Review and maintain trading order records from the order engine.', 'Order records', 'activity', '/dashboard/orders', 'order:read', 'order:write', 10, TRUE),
    ('marketKlines', 'Market', 'Market Klines', 'Inspect and manage stored candlestick data used by strategies and backfills.', 'Candlestick data', 'candlestick', '/dashboard/marketKlines', 'market:read', 'market:write', 20, TRUE),
    ('symbolMappings', 'Market', 'Symbol Mappings', 'Map internal symbols to each exchange market data and order symbols.', 'Exchange symbols', 'sliders', '/dashboard/symbolMappings', 'market:read', 'market_config:write', 30, TRUE),
    ('klineSubscriptions', 'Market', 'Kline Subscriptions', 'Control subscribed market streams and retained kline history.', 'Stream controls', 'bell', '/dashboard/klineSubscriptions', 'market:read', 'market_config:write', 40, TRUE),
    ('strategyConfigs', 'Trading', 'Strategy Configs', 'Configure runtime strategy behavior, order sizing, and risk controls.', 'Strategy behavior', 'chart', '/dashboard/strategyConfigs', 'strategy_config:read', 'strategy_config:write', 50, TRUE),
    ('settings', 'System', 'Settings', 'Manage dashboard settings and feature flags for the API service.', 'Feature flags', 'settings', '/dashboard/settings', 'settings:read', 'settings:write', 60, TRUE),
    ('users', 'Access', 'Users', 'Manage dashboard user access and account status.', 'User accounts', 'users', '/dashboard/users', 'user:read', 'user:write', 70, TRUE),
    ('roles', 'Access', 'Roles', 'Manage role-based access groups and permission assignments.', 'RBAC groups', 'shield', '/dashboard/roles', 'user:read', 'user:write', 80, TRUE),
    ('permissions', 'Access', 'Permissions', 'Manage API permission names and descriptions used by roles and page rules.', 'Permission catalog', 'key', '/dashboard/permissions', 'permission:read', 'permission:write', 85, TRUE),
    ('dashboardPages', 'System', 'Dashboard Pages', 'Manage sidebar pages, submenu grouping, ordering, and permission overrides.', 'Menu and RBAC', 'layout', '/dashboard/dashboardPages', 'dashboard_page:read', 'dashboard_page:write', 90, TRUE)
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
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE name IN ('dashboard_page:read', 'dashboard_page:write'));
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE name IN ('permission:read', 'permission:write'));
DELETE FROM permissions WHERE name IN ('dashboard_page:read', 'dashboard_page:write', 'permission:read', 'permission:write');
DROP TABLE IF EXISTS dashboard_pages;
-- +goose StatementEnd
