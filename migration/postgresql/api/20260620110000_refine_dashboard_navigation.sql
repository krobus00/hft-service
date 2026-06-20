-- +goose Up
-- +goose StatementBegin
INSERT INTO dashboard_pages (
    resource_key, parent_key, label, description, short_description, icon, path,
    read_permission, write_permission, sort_order, visible
)
VALUES
    ('orders', 'Trading', 'Orders & Positions', 'Inspect immutable exchange orders, open positions, execution state, and live PnL.', 'Execution and positions', 'activity', '/dashboard/orders', 'order:read', '', 10, TRUE),
    ('orderPnL', 'Trading', 'Trade History', 'Review closed trades with entry, exit, and running profit or loss.', 'Closed trades and PnL', 'chart', '/dashboard/orderPnL', 'order_report:read', '', 12, TRUE),
    ('dailyReports', 'Trading', 'Daily PnL', 'Review daily profit, win rate, and trade counts by strategy and symbol.', 'Daily performance', 'chart', '/dashboard/dailyReports', 'order_report:read', '', 14, TRUE),
    ('strategyPerformance', 'Trading', 'Strategy Performance', 'Compare strategy profitability, win rate, payoff, and trade quality.', 'Compare strategies', 'chart', '/dashboard/strategyPerformance', 'order_report:read', '', 16, TRUE),
    ('strategyConfigs', 'Automation', 'Strategies', 'Configure runtime strategy behavior, order sizing, and risk controls.', 'Runtime and risk controls', 'chart', '/dashboard/strategyConfigs', 'strategy_config:read', 'strategy_config:write', 18, TRUE),
    ('marketKlines', 'Market Data', 'Candles', 'Inspect immutable candlestick history consumed by strategies and backfills.', 'Stored market candles', 'candlestick', '/dashboard/marketKlines', 'market:read', '', 20, TRUE),
    ('priceReferences', 'Market Data', 'Price Feed', 'Inspect latest mark prices and update times used for open-position PnL.', 'Latest prices and freshness', 'radio', '/dashboard/priceReferences', 'market:read', '', 22, TRUE),
    ('marketBackfills', 'Market Data', 'Backfills', 'Import historical candles through the market-data service.', 'Historical candle import', 'history', '/dashboard/marketBackfills', 'market:read', 'market_config:write', 25, TRUE),
    ('symbolMappings', 'Market Data', 'Symbol Mappings', 'Manage canonical, kline, and order symbols for each exchange market.', 'Exchange symbol routing', 'sliders', '/dashboard/symbolMappings', 'market:read', 'market_config:write', 30, TRUE),
    ('klineSubscriptions', 'Market Data', 'Kline Subscriptions', 'Manage live candle streams and retained history limits.', 'Live stream controls', 'bell', '/dashboard/klineSubscriptions', 'market:read', 'market_config:write', 32, TRUE),
    ('users', 'Administration', 'Users', 'Manage dashboard accounts, status, and assigned roles.', 'Accounts and roles', 'users', '/dashboard/users', 'user:read', 'user:write', 70, TRUE),
    ('roles', 'Administration', 'Roles', 'Manage role-based access groups and permission assignments.', 'RBAC groups', 'shield', '/dashboard/roles', 'user:read', 'user:write', 72, TRUE),
    ('permissions', 'Administration', 'Permissions', 'Manage the API permission catalog used by roles.', 'Permission catalog', 'key', '/dashboard/permissions', 'permission:read', 'permission:write', 74, TRUE),
    ('settings', 'Administration', 'Settings', 'Manage dashboard settings and feature flags.', 'Dashboard settings', 'settings', '/dashboard/settings', 'settings:read', 'settings:write', 90, TRUE),
    ('dashboardPages', 'Administration', 'Navigation', 'Manage sidebar labels, grouping, visibility, and permission overrides.', 'Menu configuration', 'layout', '/dashboard/dashboardPages', 'dashboard_page:read', 'dashboard_page:write', 92, TRUE)
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

DELETE FROM permissions WHERE name IN ('order:write', 'market:write');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM dashboard_pages
WHERE resource_key IN ('orderPnL', 'dailyReports', 'strategyPerformance', 'priceReferences', 'marketBackfills');

UPDATE dashboard_pages SET parent_key = 'Trading', label = 'Orders', write_permission = 'order:write', sort_order = 10 WHERE resource_key = 'orders';
UPDATE dashboard_pages SET parent_key = 'Market', label = 'Market Klines', write_permission = 'market:write', sort_order = 20 WHERE resource_key = 'marketKlines';
UPDATE dashboard_pages SET parent_key = 'Market', sort_order = 30 WHERE resource_key = 'symbolMappings';
UPDATE dashboard_pages SET parent_key = 'Market', sort_order = 40 WHERE resource_key = 'klineSubscriptions';
UPDATE dashboard_pages SET parent_key = 'Trading', label = 'Strategy Configs', sort_order = 50 WHERE resource_key = 'strategyConfigs';
UPDATE dashboard_pages SET parent_key = 'System', sort_order = 60 WHERE resource_key = 'settings';
UPDATE dashboard_pages SET parent_key = 'Access', sort_order = 70 WHERE resource_key = 'users';
UPDATE dashboard_pages SET parent_key = 'Access', sort_order = 80 WHERE resource_key = 'roles';
UPDATE dashboard_pages SET parent_key = 'Access', sort_order = 85 WHERE resource_key = 'permissions';
UPDATE dashboard_pages SET parent_key = 'System', label = 'Dashboard Pages', sort_order = 90 WHERE resource_key = 'dashboardPages';

INSERT INTO permissions (name, description)
VALUES
    ('order:write', 'Create, update, and delete order history'),
    ('market:write', 'Create, update, and delete market kline data')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name IN ('order:write', 'market:write')
WHERE r.name = 'admin'
ON CONFLICT DO NOTHING;
-- +goose StatementEnd
