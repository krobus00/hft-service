# API Standard

The `api` service is the centralized HTTP API for dashboard clients.

Base URL:

```text
http://localhost:9804/api/v1
```

## Response Envelope

Every endpoint returns the same response shape:

```json
{
  "message": "success",
  "code": "SUCCESS",
  "data": {}
}
```

Error response:

```json
{
  "message": "invalid authorization token",
  "code": "UNAUTHORIZED"
}
```

Validation errors, when used, are returned as:

```json
{
  "message": "validation failed",
  "code": "BAD_REQUEST",
  "validation_errors": [
    {
      "field": "email",
      "tag": "required",
      "message": "email is required"
    }
  ]
}
```

## Pagination Response

Paginated endpoints put pagination metadata under `data.meta` and rows under `data.items`:

```json
{
  "message": "success",
  "code": "SUCCESS",
  "data": {
    "meta": {
      "page": 1,
      "limit": 10,
      "totalItems": 100,
      "totalPages": 10
    },
    "items": []
  }
}
```

## Query Parameters

List endpoints support these query parameters:

```text
filter=[{"field":"exchange","op":"eq","value":["binance"]}]
pagination={"page":1,"limit":10}
sort={"field":"created_at","direction":"desc"}
keyword=BTC
```

Supported filter operators:

```text
eq, neq, in, contain, between, gte, lte, gt, lt
```

`pagination.limit` defaults to `10` and is capped at `100`.

## Authentication

Create the first admin user once:

```http
POST /api/v1/auth/setup
Content-Type: application/json

{
  "email": "admin@example.com",
  "name": "Admin",
  "password": "change-me"
}
```

Login:

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "admin@example.com",
  "password": "change-me"
}
```

Authenticated requests use:

```http
Authorization: Bearer <access_token>
```

Refresh an access token:

```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "<refresh_token>"
}
```

Logout revokes the refresh token:

```http
POST /api/v1/auth/logout
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "refresh_token": "<refresh_token>"
}
```

Get current authenticated user:

```http
GET /api/v1/auth/me
Authorization: Bearer <access_token>
```

## RBAC

Seeded roles:

- `admin`: all permissions.
- `viewer`: read-only dashboard permissions.

Seeded permissions:

- `order:read`
- `market:read`
- `market_config:write`
- `strategy_config:read`
- `strategy_config:write`
- `settings:read`
- `settings:write`
- `user:read`
- `user:write`

## Endpoints

Auth:

- `POST /api/v1/auth/setup`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`

Orders:

- `GET /api/v1/orders`
- `GET /api/v1/orders/{id}`

Market data:

- `GET /api/v1/market/klines`
- `GET /api/v1/market/klines/{open_time}`

Market config:

- `GET /api/v1/market/symbol-mappings`
- `POST /api/v1/market/symbol-mappings`
- `GET /api/v1/market/symbol-mappings/{id}`
- `PUT /api/v1/market/symbol-mappings/{id}`
- `DELETE /api/v1/market/symbol-mappings/{id}`
- `GET /api/v1/market/kline-subscriptions`
- `POST /api/v1/market/kline-subscriptions`
- `GET /api/v1/market/kline-subscriptions/{id}`
- `PUT /api/v1/market/kline-subscriptions/{id}`
- `DELETE /api/v1/market/kline-subscriptions/{id}`

Strategy config:

- `GET /api/v1/strategy/configs`
- `POST /api/v1/strategy/configs`
- `GET /api/v1/strategy/configs/{id}`
- `PUT /api/v1/strategy/configs/{id}`
- `DELETE /api/v1/strategy/configs/{id}`

API settings:

- `GET /api/v1/settings`
- `POST /api/v1/settings`
- `GET /api/v1/settings/{id}`
- `PUT /api/v1/settings/{id}`
- `DELETE /api/v1/settings/{id}`

Users and roles:

- `GET /api/v1/users`
- `GET /api/v1/users/{id}`
- `GET /api/v1/roles`
- `GET /api/v1/roles/{id}`

## Database

The API service uses:

- `database.api` for dashboard auth, RBAC, refresh tokens, and API settings.
- `database.market_data` for market data and strategy config.
- `database.order_engine` for order history.

Run migrations:

```bash
go run . migrate --databaseName=api
go run . migrate --databaseName=market_data
go run . migrate --databaseName=order_engine
```
