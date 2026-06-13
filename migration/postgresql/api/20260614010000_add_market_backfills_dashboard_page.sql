-- +goose Up
-- +goose StatementBegin
INSERT INTO dashboard_pages (resource_key, parent_key, label, description, short_description, icon, path, read_permission, write_permission, sort_order, visible)
VALUES (
    'marketBackfills',
    'Market',
    'Backfills',
    'Run historical market kline backfills through the market-data gRPC service.',
    'Historical import',
    'database',
    '/dashboard/marketBackfills',
    'market:read',
    'market_config:write',
    25,
    TRUE
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
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM dashboard_pages WHERE resource_key = 'marketBackfills';
-- +goose StatementEnd
