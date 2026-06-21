# API Service

The `api` service is the centralized HTTP provider for the future dashboard. It exposes authentication, RBAC-protected dashboard data, order history, market data, and configurable trading/runtime data stored in Postgres.

## Runtime

Command:

```bash
go run . api
```

Default local base URL:

```text
http://localhost:9804/api/v1
```

Docker Compose service:

```bash
docker compose --profile infra --profile app up api
```

Required config keys:

```yaml
dashboard_auth:
  issuer: hft-dashboard
  token_secret: REPLACE_WITH_DASHBOARD_SECRET
  access_token_ttl: 15m
  refresh_token_ttl: 168h
port:
  api_http: 9804
database:
  api:
    dsn: "postgres://root:root@postgresql:5432/api?sslmode=disable"
  market_data:
    dsn: "postgres://root:root@postgresql:5432/market_data?sslmode=disable"
  order_engine:
    dsn: "postgres://root:root@postgresql:5432/order_engine?sslmode=disable"
redis:
  api:
    cache_dsn: "redis://redis:6379/2"
    default_cache_duration: 5m
```

## Database

The service uses three database connections:

- `api`: users, roles, permissions, refresh tokens, and dashboard settings.
- `market_data`: market klines, symbol mappings, kline subscriptions, and strategy configs.
- `order_engine`: order history.

Run migrations:

```bash
go run . migrate --databaseName=api
go run . migrate --databaseName=market_data
go run . migrate --databaseName=order_engine
```

The `api` migration creates:

- `users`
- `roles`
- `permissions`
- `user_roles`
- `role_permissions`
- `settings`

It seeds `admin` and `viewer` roles plus the standard dashboard permissions.

## Redis Auth Cache

The API service uses `redis.api` to cache authenticated user profile data:

- user profile cache key: `hft:api:auth:user:{user_id}`
- refresh session key: `hft:api:auth:refresh:{refresh_token_hash}`
- cached user data: user ID, email, name, roles, and permissions
- default TTL: `redis.api.default_cache_duration`

`GET /api/v1/auth/me` is read-through cached. Refresh tokens are stored only in Redis, not Postgres. Login and setup create a Redis refresh session. Refresh rotates the refresh token by deleting the old Redis key and storing a new one with `dashboard_auth.refresh_token_ttl`. Logout deletes the Redis refresh session.

Postgres remains the source of truth for users, roles, and permissions. Redis is the source of truth for active refresh sessions.

## Authentication

Create the first admin user once:

```bash
curl -X POST http://localhost:9804/api/v1/auth/setup \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","name":"Admin","password":"change-me"}'
```

Login:

```bash
curl -X POST http://localhost:9804/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"change-me"}'
```

Use the returned access token:

```http
Authorization: Bearer <access_token>
```

Refresh token:

```bash
curl -X POST http://localhost:9804/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh_token>"}'
```

Logout revokes the refresh token:

```bash
curl -X POST http://localhost:9804/api/v1/auth/logout \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh_token>"}'
```

Get the current authenticated user:

```bash
curl http://localhost:9804/api/v1/auth/me \
  -H "Authorization: Bearer <access_token>"
```

## Query Standard

All list endpoints support:

```text
filter=[{"field":"exchange","op":"eq","value":["binance"]}]
pagination={"page":1,"limit":10}
sort={"field":"created_at","direction":"desc"}
keyword=BTC
```

Supported operators:

```text
eq, neq, in, contain, between, gte, lte, gt, lt
```

Example:

```bash
curl "http://localhost:9804/api/v1/strategy/configs?pagination={\"page\":1,\"limit\":10}&sort={\"field\":\"created_at\",\"direction\":\"desc\"}&keyword=BTC" \
  -H "Authorization: Bearer <access_token>"
```

See [API_STANDARD.md](./API_STANDARD.md) for the common response envelope and pagination response format.

## RBAC

Roles:

- `admin`: all dashboard permissions.
- `viewer`: read-only permissions.

Permissions:

- `order:read`
- `market:read`
- `market_config:write`
- `strategy_config:read`
- `strategy_config:write`
- `settings:read`
- `settings:write`
- `user:read`
- `user:write`

Write operations require the matching write permission. `admin` bypasses individual permission checks.

## Endpoints

### Auth

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `POST` | `/api/v1/auth/setup` | No | Create the first admin user. Fails after a user exists. |
| `POST` | `/api/v1/auth/login` | No | Login with email and password. |
| `POST` | `/api/v1/auth/refresh` | No | Rotate refresh token and issue a new access token. |
| `POST` | `/api/v1/auth/logout` | Yes | Revoke a refresh token. |
| `GET` | `/api/v1/auth/me` | Yes | Return current user profile, roles, and permissions. |

### Orders

Requires `order:read`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/v1/orders` | Paginated order history from `order_histories`. |
| `GET` | `/api/v1/orders/{id}` | Order history detail. |

Common filter fields:

```text
id, request_id, user_id, exchange, market_type, position_side, symbol, order_id,
entry_order_id, side, type, status, strategy_id, trade_condition,
is_paper_trading, created_at, updated_at
```

`GET /api/v1/order-reports/strategy-performance` requires `order_report:read`. It uses paired trade PnL for ENTRY/EXIT strategies and cashflow plus latest inventory value for unpaired spot SIGNAL strategies such as `python-micro-grid`.

### Market Klines

Requires `market:read`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/v1/market/klines` | Paginated market kline data. |
| `GET` | `/api/v1/market/klines/{open_time}` | Kline detail by `open_time` text value. |

Common filter fields:

```text
exchange, market_type, event_type, symbol, interval, open_time, close_time,
is_closed, created_at, updated_at
```

### Symbol Mappings

Read requires `market:read`. Create, update, and delete require `market_config:write`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/v1/market/symbol-mappings` | Paginated symbol mappings. |
| `POST` | `/api/v1/market/symbol-mappings` | Create a symbol mapping. |
| `GET` | `/api/v1/market/symbol-mappings/{id}` | Symbol mapping detail. |
| `PUT/PATCH` | `/api/v1/market/symbol-mappings/{id}` | Update a symbol mapping. |
| `DELETE` | `/api/v1/market/symbol-mappings/{id}` | Delete a symbol mapping. |

Body:

```json
{
  "exchange": "binance",
  "market_type": "futures",
  "symbol": "BTC_USDT",
  "kline_symbol": "btcusdt",
  "order_symbol": "BTCUSDT"
}
```

### Kline Subscriptions

Read requires `market:read`. Create, update, and delete require `market_config:write`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/v1/market/kline-subscriptions` | Paginated kline subscriptions. |
| `POST` | `/api/v1/market/kline-subscriptions` | Create a kline subscription. |
| `GET` | `/api/v1/market/kline-subscriptions/{id}` | Subscription detail. |
| `PUT/PATCH` | `/api/v1/market/kline-subscriptions/{id}` | Update a subscription. |
| `DELETE` | `/api/v1/market/kline-subscriptions/{id}` | Delete a subscription. |

Body:

```json
{
  "exchange": "binance",
  "market_type": "futures",
  "symbol": "BTC_USDT",
  "interval": "1m",
  "payload": {
    "source": "dashboard"
  },
  "max_kline_data": 1000
}
```

### Strategy Configs

Read requires `strategy_config:read`. Create, update, and delete require `strategy_config:write`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/v1/strategy/configs` | Paginated strategy configs. |
| `POST` | `/api/v1/strategy/configs` | Create a strategy config row. |
| `GET` | `/api/v1/strategy/configs/{id}` | Strategy config detail. |
| `PUT/PATCH` | `/api/v1/strategy/configs/{id}` | Update a strategy config row. |
| `DELETE` | `/api/v1/strategy/configs/{id}` | Delete a strategy config row. |

Body:

```json
{
  "strategy": "python-krobot01-ema200-vwap-macd",
  "exchange": "binance",
  "market_type": "futures",
  "symbol": "BTC_USDT",
  "interval": "1m",
  "user_id": "minimax-01",
  "position_side": "BOTH",
  "source": "dashboard",
  "need_notification": true,
  "is_paper_trading": true,
  "order_type": "MARKET",
  "order_qty": "10",
  "limit_slippage_pct": "0.02",
  "cooldown_bars": 2,
  "sl_cooldown_bars": 3,
  "max_consecutive_stop_losses": 2,
  "sl_pause_bars": 10,
  "take_profit_pct": "0.25",
  "stop_loss_pct": "0.15",
  "trailing_stop_pct": "0.12",
  "trailing_stop_trigger_pct": "0.20",
  "max_hold_bars": 24,
  "max_positions": 1,
  "enable_intrabar_risk_exit": true
}
```

### Settings

Read requires `settings:read`. Create, update, and delete require `settings:write`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/v1/settings` | Paginated dashboard settings. |
| `POST` | `/api/v1/settings` | Create a setting. |
| `GET` | `/api/v1/settings/{id}` | Setting detail. |
| `PUT/PATCH` | `/api/v1/settings/{id}` | Update a setting. |
| `DELETE` | `/api/v1/settings/{id}` | Delete a setting. |

Body:

```json
{
  "key": "dashboard.theme",
  "value": {
    "mode": "dark"
  }
}
```

### Users And Roles

Requires `user:read`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/v1/users` | Paginated users. |
| `GET` | `/api/v1/users/{id}` | User detail. |
| `GET` | `/api/v1/roles` | Paginated roles. |
| `GET` | `/api/v1/roles/{id}` | Role detail. |

## Error Codes

Common `code` values:

- `SUCCESS`
- `BAD_REQUEST`
- `UNAUTHORIZED`
- `FORBIDDEN`
- `NOT_FOUND`
- `CONFLICT`
- `INTERNAL_ERROR`

## Implementation Notes

- The API service uses the existing handler, service, and repository layers.
- Repository methods are domain-specific. There is no generic table repository or generic resource dispatcher.
- All mutable dashboard configuration is stored in Postgres.
- Historical order and market kline endpoints are read-only.
