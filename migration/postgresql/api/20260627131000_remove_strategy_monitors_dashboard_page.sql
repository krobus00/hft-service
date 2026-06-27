-- +goose Up
DELETE FROM dashboard_pages WHERE resource_key = 'strategyMonitors';

-- +goose Down
INSERT INTO dashboard_pages (
    resource_key, parent_key, label, description, short_description, icon, path,
    read_permission, write_permission, sort_order, visible, created_at, updated_at
)
VALUES (
    'strategyMonitors', 'Automation', 'Strategy Monitors',
    'Watch live strategy runner health, state, metrics, and restart/reset controls.',
    'Runner health and state', 'activity', '/dashboard/strategyMonitors',
    'strategy_config:read', 'strategy_config:write', 19, TRUE, NOW(), NOW()
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
