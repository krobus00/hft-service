-- +goose Up
INSERT INTO dashboard_pages (
    resource_key, parent_key, label, description, short_description, icon, path,
    read_permission, write_permission, sort_order, visible, created_at, updated_at
)
VALUES (
    'strategyMetrics', 'Automation', 'Strategy Detail',
    'Inspect strategy candles, indicators, order markers, and execution metrics by symbol.',
    'Candles, indicators, orders', 'chart', '/dashboard/strategyMetrics',
    'strategy_config:read', '', 17, TRUE, NOW(), NOW()
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
    visible = EXCLUDED.visible,
    updated_at = NOW();

-- +goose Down
DELETE FROM dashboard_pages WHERE resource_key = 'strategyMetrics';
