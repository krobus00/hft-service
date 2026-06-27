-- +goose Up
INSERT INTO permissions (name, description)
VALUES
    ('market:read', 'Read market data and indicator configuration'),
    ('market_config:write', 'Manage market data and indicator configuration')
ON CONFLICT (name) DO NOTHING;

INSERT INTO dashboard_pages (
    resource_key, parent_key, label, description, short_description, icon, path,
    read_permission, write_permission, sort_order, visible
)
VALUES (
    'indicatorConfigs', 'Market Data', 'Indicator Configs',
    'Configure shared indicator calculations for strategy candle events.',
    'Shared indicator settings', 'chart', '/dashboard/indicatorConfigs',
    'market:read', 'market_config:write', 34, TRUE
)
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
    visible = EXCLUDED.visible;

-- +goose Down
DELETE FROM dashboard_pages WHERE resource_key = 'indicatorConfigs';
