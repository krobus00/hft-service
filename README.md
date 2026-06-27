# High Frequency Trading Service

Event-driven trading platform for market data ingestion, strategy execution, and order execution.

## Why This Project Exists

This repository is built to run automated crypto trading workflows in a modular, production-oriented architecture:

- ingest real-time exchange kline streams,
- execute strategy signals,
- place and track orders,
- and send operational alerts.

Most of the code in this repository was written with AI assistance.

## Table of Contents

- [How HFT Works in This Repository](#how-hft-works-in-this-repository)
- [System Architecture](#system-architecture)
- [Services and Use Cases](#services-and-use-cases)
- [API Service Documentation](docs/API_SERVICE.md)
- [Dashboard](#dashboard)
- [Supported Exchanges](#supported-exchanges)
- [Market Type Support](#market-type-support)
- [Quick Start (Local)](#quick-start-local)
- [Production Docker Compose](#production-docker-compose)
- [Database Backups](#database-backups)
- [How to Run Strategy](#how-to-run-strategy)
- [Profit Results](#profit-results)
- [Test Order API](#test-order-api)
- [Roadmap](#roadmap)
- [Support This Project](#support-this-project)
- [Contributing](#contributing)

## How HFT Works in This Repository

This project follows an event-driven HFT loop:

1. Market data gateway subscribes to exchange websocket kline streams.
2. Incoming candles are normalized and published to NATS JetStream (`KLINE.<EXCHANGE>.<SYMBOL>.<INTERVAL>`).
3. Worker persists market data into the `market_data` database.
4. Indicator service consumes raw kline events, calculates enabled `indicator_configs` with `github.com/cinar/indicator/v2`, and publishes shared values to `KLINE_INDICATOR.<EXCHANGE>.<SYMBOL>.<INTERVAL>`.
5. `strategy-service` consumes enriched closed candles and decides buy/sell/hold from web/API-configured rules.
6. If action is needed, strategy publishes `order_engine.place_order`.
7. Order engine gateway executes the order to the exchange.
8. Order engine worker continuously reconciles order status until terminal state.
9. Notification service emits Discord alerts for important events.

In short: low-latency event ingestion + deterministic signal execution + robust post-trade reconciliation.

## System Architecture

### Event Flow

1. `market-data-gateway` consumes exchange kline data.
2. `market-data-gateway` publishes `KLINE` events.
3. `market-data-worker` consumes `KLINE` and stores records in `market_data` DB.
4. Indicator service consumes `KLINE` and publishes enriched `KLINE_INDICATOR` events.
5. `strategy-service` consumes `KLINE_INDICATOR` and decides: buy, sell, or no action.
6. If execution is required, `strategy` publishes `order_engine.place_order`.
7. `order-engine-gateway` consumes order events and executes to exchange.
8. `order-engine-gateway` stores initial order history in `order_engine` DB.
9. `order-engine-worker` polls tracked orders and fetches latest execution state from exchange.
10. `order-engine-worker` updates `order_histories` status in `order_engine` DB.
11. `notification-service` consumes notification events and sends Discord alerts.

### Diagram

```mermaid
flowchart TB
  EX[(Exchange)]
  NATS[(NATS JetStream)]
  MDG[market-data-gateway]
  MDW[market-data-worker]
  IND[indicator service]
  STRAT[strategy-service]
  OEG[order-engine-gateway]
  OEW[order-engine-worker]
  NS[notification-service]
  MDB[(market_data DB)]
  ODB[(order_engine DB)]
  DISCORD[(Discord)]

  EX -->|kline websocket| MDG
  MDG -->|publish KLINE| NATS
  NATS -->|consume KLINE| MDW
  NATS -->|consume KLINE| IND
  IND -->|publish KLINE_INDICATOR| NATS
  NATS -->|consume KLINE_INDICATOR| STRAT
  MDW -->|insert kline| MDB
  STRAT -->|publish order_engine.place_order| NATS
  NATS -->|consume place_order| OEG
  OEG -->|execute order| EX
  OEG -->|store order history| ODB
  OEG -->|publish order_engine.notification_alert| NATS
  NATS -->|consume notification_alert| NS
  NS -->|send alert| DISCORD
  ODB -->|load tracked orders| OEW
  OEW -->|fetch latest order status| EX
  OEW -->|update order_histories status| ODB
```

## Services and Use Cases

### `market-data-gateway`

- Connects to exchange websocket stream.
- Normalizes incoming kline payload.
- Resolves exchange kline symbols into canonical internal symbols from `symbol_mappings.symbol`.
- For Binance, runs spot and futures websocket subscriptions in parallel by default.
- Publishes `KLINE.<EXCHANGE>.<SYMBOL>.<INTERVAL>` to JetStream.

### `market-data-worker`

- Consumes `KLINE` events.
- Persists market kline records using canonical internal symbol.
- Retries failed processing (bounded by config).
- Calculates shared indicator values from `indicator_configs` and publishes enriched kline events for strategies.

### `strategy-service`

- Go strategy runner for web/API-built rules.
- Consumes `KLINE_INDICATOR` events.
- Loads enabled `strategy_configs` and `strategy_rules`.
- Publishes `order_engine.place_order` when rule conditions match.
- Supports condition operators `gt`, `gte`, `lt`, `lte`, `eq`, `neq`, `cross_above`, and `cross_below`.
- Horizontally scales with NATS queue subscription; run multiple replicas of the same service.

### `order-engine-gateway`

- Consumes place-order events.
- Converts canonical internal symbol to exchange order symbol using `symbol_mappings`.
- Routes Binance orders to spot/futures endpoint by `market_type`.
- Sends order and stores order history.

### `order-engine-worker`

- Syncs `order_histories` records with the latest execution state from exchange.
- Fetches latest order status from exchange for tracked orders.
- Updates local order history status to keep it consistent with exchange execution result.

### `notification-service`

- Consumes `order_engine.notification_alert` events from JetStream.
- Sends Discord alert messages for orders with `need_notification = true`.
- Alert payload is general (no quantity), containing strategy name, symbol, internal symbol, interval, side, and price.

### `api`

- Provides the centralized dashboard HTTP API.
- Handles dashboard authentication, refresh tokens, and RBAC.
- Exposes read APIs for orders and market data.
- Exposes CRUD APIs for symbol mappings, kline subscriptions, indicator configs, strategy configs, strategy rules, and dashboard settings.
- Full endpoint documentation: [docs/API_SERVICE.md](docs/API_SERVICE.md).

### `migrate`

- Runs DB migration for `api`, `market_data`, and `order_engine`.

## Dashboard

The dashboard is ready and lives in [`web/`](web/). It is a Next.js TypeScript app for operating the Krobot API service.

Dashboard capabilities:

- First-admin setup flow at `/setup`.
- Login, refresh-token session handling, and logout.
- RBAC-aware navigation and protected dashboard pages.
- Orders, market data, symbol mappings, kline subscriptions, indicator configs, strategy configs, dashboard settings, users, roles, permissions, and dashboard page management.

### Screenshot

![Dashboard auth](docs/dashboard-login.png)

![Dashboard screenshot](docs/dashboard-screenshot.png)

### Local Dashboard Setup

Start the infrastructure and API dependencies first:

```bash
docker compose -f compose-dev.yaml up -d postgresql redis nats
```

Create the API database if it does not already exist:

```sql
CREATE DATABASE api;
```

Run API migrations:

```bash
go run . migrate --databaseName=api
```

Start the API service:

```bash
go run . api
```

In another terminal, start the dashboard:

```bash
cd web
npm install
npm run dev
```

PowerShell on some Windows machines blocks `npm.ps1`; use `npm.cmd install` and `npm.cmd run dev` if needed.

The dashboard uses this API base URL by default:

```bash
NEXT_PUBLIC_API_BASE_URL=http://localhost:9804/api/v1
```

Open:

- Dashboard setup: `http://localhost:3000/setup`
- Dashboard login/app: `http://localhost:3000`

Production and local Docker Compose include the web service through the `web` profile. The default ports are:

- API: `http://localhost:9804/api/v1`
- Dashboard: `http://localhost:3000`

## Supported Exchanges

- Tokocrypto
- Binance

## Market Type Support

- Supported values: `spot`, `futures`.
- `spot` is the default when `market_type` is omitted.
- Binance supports both spot and futures simultaneously by default.
- Tokocrypto currently uses `spot`.

## Quick Start (Local)

### 1) Prerequisites

- Go `1.25+`
- Docker + Docker Compose

### 2) Configure app

Copy config template:

```bash
copy config.yml.example config.yml
```

Copy environment template for Docker Compose and Makefile defaults:

```bash
copy .env.example .env
```

Update required values in `config.yml`:

- `exchanges.binance.api_key`
- `exchanges.binance.api_secret`
- `exchanges.tokocrypto.api_key`
- `exchanges.tokocrypto.api_secret`
- `api_keys[*].key`
- `notification.discord_webhook_url` (required when running `notification-service`)

If you run multiple users/API keys, configure per-user exchange accounts (recommended):

```yaml
exchanges:
  binance:
    api_key: "default-key-optional"
    api_secret: "default-secret-optional"
    accounts:
      paper-1:
        api_key: "binance-key-user-1"
        api_secret: "binance-secret-user-1"
      minimax-01:
        api_key: "binance-key-user-2"
        api_secret: "binance-secret-user-2"
  tokocrypto:
    api_key: "default-key-optional"
    api_secret: "default-secret-optional"
    accounts:
      paper-1:
        api_key: "tokocrypto-key-user-1"
        api_secret: "tokocrypto-secret-user-1"
```

Credential resolution behavior:

- Order engine resolves exchange credentials by `user_id` first from `exchanges.<exchange>.accounts.<user_id>`.
- If no account match is found, it falls back to `exchanges.<exchange>.api_key/api_secret`.

Host mapping reminder:

- Run app locally (`go run`): use `localhost` hosts.
- Run app in Docker: use compose service names (`postgresql`, `redis`, `nats`).

Optional Redis/NATS authentication:

- If Docker Compose starts Redis with `REDIS_PASSWORD`, update every Redis DSN to include that password.
- If Docker Compose starts NATS with `NATS_PASSWORD`, update every NATS URL to include the configured user and password.

Docker service URL examples:

```yaml
nats_jetstream:
  url: "nats://hft:REPLACE_WITH_NATS_PASSWORD@nats:4222"

redis:
  api:
    cache_dsn: "redis://:REPLACE_WITH_REDIS_PASSWORD@redis:6379/2"
  market_data:
    cache_dsn: "redis://:REPLACE_WITH_REDIS_PASSWORD@redis:6379/0"
  strategy:
    cache_dsn: "redis://:REPLACE_WITH_REDIS_PASSWORD@redis:6379/1"
```

### 3) Start infrastructure

```bash
docker compose -f compose-dev.yaml up -d postgresql redis nats
```

### 4) Create databases

Run in PostgreSQL:

```sql
CREATE DATABASE api;
CREATE DATABASE market_data;
CREATE DATABASE order_engine;
```

### 5) Run migrations

```bash
go run . migrate --databaseName=api
go run . migrate --databaseName=market_data
go run . migrate --databaseName=order_engine
```

### 6) Seed `market_data`

Seed at least one subscription and symbol mapping per exchange you want to run.

Symbol rules:

- `symbol_mappings.symbol` is the canonical internal symbol used by the service.
- `kline_subscriptions.symbol` must use the same canonical internal symbol.
- `symbol_mappings.market_type` and `kline_subscriptions.market_type` must match (`spot` or `futures`).
- Subscription `payload` is regenerated by the gateway from mapping (`kline_symbol`) and interval.

#### Binance example (BTCUSDT, 1m)

```sql
INSERT INTO kline_subscriptions
(id, exchange, market_type, symbol, "interval", payload, created_at, updated_at)
VALUES
('3f040de8-c018-4f74-9ad8-7bcfe6f11db1', 'binance', 'spot', 'BTC_USDT', '1m', '{"method": "SUBSCRIBE", "params": ["btcusdt@kline_1m"], "id": 1}'::jsonb, NOW(), NOW()),
('2f7690af-bebf-4c66-aec3-7ea4c4feccbf', 'binance', 'futures', 'BTC_USDT', '1m', '{"method": "SUBSCRIBE", "params": ["btcusdt@kline_1m"], "id": 1}'::jsonb, NOW(), NOW());

INSERT INTO symbol_mappings
(id, exchange, market_type, symbol, kline_symbol, order_symbol, created_at, updated_at)
VALUES
('f2e7bca4-d3f7-4b5f-a74f-5273f36ea565', 'binance', 'spot', 'BTC_USDT', 'btcusdt', 'BTCUSDT', NOW(), NOW()),
('a9f00eab-9b1f-4b18-858f-4e5aa12f741c', 'binance', 'futures', 'BTC_USDT', 'btcusdt', 'BTCUSDT', NOW(), NOW());
```

#### Tokocrypto example (TKOIDR, 1m)

```sql
INSERT INTO kline_subscriptions
(id, exchange, market_type, symbol, "interval", payload, created_at, updated_at)
VALUES
('2675c22f-d94b-4ede-bb27-5b6c375a1089', 'tokocrypto', 'spot', 'TKO_IDR', '1m', '{"id": 1, "method": "SUBSCRIBE", "params": ["tkoidr@kline_1m"]}'::jsonb, '2026-02-22 13:46:45.903', '2026-02-22 13:46:45.903');

INSERT INTO symbol_mappings
(id, exchange, market_type, symbol, kline_symbol, order_symbol, created_at, updated_at)
VALUES
('4494faed-468c-46b5-b7ca-389419ad63ad', 'tokocrypto', 'spot', 'TKO_IDR', 'tkoidr', 'TKO_IDR', '2026-02-22 14:36:22.978', '2026-02-22 14:36:22.978');
```

### 7) Seed `strategy_configs` (required)

`user_id`, order sizing, and risk controls are sourced per pair from this table.

Example:

```sql
INSERT INTO strategy_configs
(id, strategy, exchange, market_type, symbol, interval, user_id, need_notification, is_paper_trading, order_type, order_qty, limit_slippage_pct,
 cooldown_bars, sl_cooldown_bars, max_consecutive_stop_losses, sl_pause_bars, take_profit_pct, stop_loss_pct, trailing_stop_pct, trailing_stop_trigger_pct, max_hold_bars, max_positions, enable_intrabar_risk_exit,
 created_at, updated_at)
VALUES
('7f5f6d39-fd5b-4c94-b9f4-41c9b92e2c01', 'go-ema-cross', 'tokocrypto', 'spot', 'TKO_IDR', '1m', 'paper-1', true, true, 'LIMIT', 10, 0.02,
 2, 3, 2, 10, 0.25, 0.15, 0.12, 0.20, 24, 1, true,
 NOW(), NOW()),
('f0e8d75f-77de-448b-9eb3-9948e3a0d742', 'go-vwap-volume', 'tokocrypto', 'spot', 'TKO_IDR', '1m', 'paper-02', true, true, 'LIMIT', 10, 0.02,
 2, 3, 2, 10, 0.25, 0.15, 0.12, 0.20, 24, 1, true,
 NOW(), NOW()),
('96c50cef-07ea-42e2-b4f7-bf9f12c11a82', 'go-btc-futures', 'binance', 'futures', 'BTC_USDT', '1m', 'minimax-01', true, false, 'MARKET', 10, 0.02,
 2, 3, 2, 10, 0.25, 0.15, 0.12, 0.20, 24, 1, true,
 NOW(), NOW()),
('a0320598-131f-4b44-8400-7ab1810bfc50', 'go-capital-guard', 'binance', 'futures', 'BTC_USDT', '1m', 'paper-1', true, true, 'LIMIT', 10, 0.02,
 2, 5, 2, 30, 0.35, 0.18, 0.12, 0.18, 18, 1, true,
 NOW(), NOW());
```

Important rules:

- `strategy` is the strategy name used in orders and reports.
- `exchange/market_type/symbol/interval` must match active kline subscription + symbol mapping.
- `user_id` must match your exchange account key path, for example `exchanges.binance.accounts.minimax-01`.

### 8) Configure strategy rules

Create `indicator_configs` and `strategy_rules` from the dashboard or API. A rule condition can reference candle fields (`close`, `high`, `low`, `volume`) and shared indicators (`indicators.<output_name>`).

### 9) Run services (separate terminals)

```bash
go run . api
```

```bash
go run . market-data-gateway
```

```bash
go run . market-data-worker
```

Run Go strategy rules built through API/web:

```bash
go run . strategy-service
```

```bash
go run . order-engine-gateway
```

```bash
go run . order-engine-worker
```

```bash
go run . notification-service
```

```bash
go run . strategy-service
```

## Production Docker Compose

The production [compose.yaml](compose.yaml) does not build local application images. It runs images already released to Docker Hub under `krobus00`.

Copy and edit `.env` first:

```bash
copy .env.example .env
```

Docker Compose reads `.env` automatically. The Makefile also includes `.env` when it exists, so image tags, ports, credentials, profiles, and backup settings can live in one place.

Default image tags:

- `krobus00/hft-service:${HFT_VERSION:-latest}`
- `krobus00/krobot-web:${WEB_VERSION:-latest}`

Build and push release images:

```bash
make build-images VERSION=1.0.0 NEXT_PUBLIC_API_BASE_URL=http://YOUR_HOST:9804/api/v1
make push-images VERSION=1.0.0
```

Run released images:

Set `HFT_VERSION` and `WEB_VERSION` in `.env`, then run:

```bash
make compose-pull
make up-service
```

Default profiles are `infra app web`. The app profile includes the Go `strategy-service`.

Scale the strategy executor horizontally:

```bash
docker compose --profile infra --profile app up -d --scale strategy-service=3
```

Redis and NATS passwords are optional. If you set them in Compose, also update `config.yml` URLs to match.

Set these in `.env`:

```env
REDIS_PASSWORD=REPLACE_WITH_REDIS_PASSWORD
NATS_USER=hft
NATS_PASSWORD=REPLACE_WITH_NATS_PASSWORD
```

Credentialed URL formats:

```yaml
nats_jetstream:
  url: "nats://hft:REPLACE_WITH_NATS_PASSWORD@nats:4222"

redis:
  api:
    cache_dsn: "redis://:REPLACE_WITH_REDIS_PASSWORD@redis:6379/2"
```

For local development infrastructure, the same password variables work with `compose-dev.yaml`:

```bash
REDIS_PASSWORD='REPLACE_WITH_REDIS_PASSWORD' \
NATS_USER=hft \
NATS_PASSWORD='REPLACE_WITH_NATS_PASSWORD' \
docker compose -f compose-dev.yaml up -d postgresql redis nats
```

Useful production commands:

```bash
make compose-config
make compose-logs
make down-service
```

For local images that already exist on your machine, use [compose-local.yaml](compose-local.yaml):

```bash
make build-local-images
make up-local-service
```

Local image names can be overridden in `.env`:

```env
HFT_LOCAL_IMAGE=hft-service:local
WEB_LOCAL_IMAGE=krobot-web:local
```

## Database Backups

Backups are written to `backups/`, which is ignored by git.

Back up one database:

```bash
make up-service
make backup-db DB=api
```

Back up all configured databases:

```bash
make backup-databases
```

If the database is running from the dev Compose file instead, point the backup target at it:

```bash
docker compose -f compose-dev.yaml up -d postgresql
make backup-databases BACKUP_COMPOSE_FILE=compose-dev.yaml
```

For remote PostgreSQL, set `POSTGRES_HOST`; `pg_dump` runs from a temporary PostgreSQL Docker image:

```bash
make backup-db \
  POSTGRES_HOST=db.example.com \
  POSTGRES_PORT=5432 \
  POSTGRES_USER=root \
  POSTGRES_PASSWORD='REPLACE_WITH_PASSWORD' \
  POSTGRES_SSLMODE=require \
  DB=api
```

Back up multiple remote databases:

```bash
make backup-databases \
  POSTGRES_HOST=db.example.com \
  POSTGRES_USER=root \
  POSTGRES_PASSWORD='REPLACE_WITH_PASSWORD' \
  POSTGRES_SSLMODE=require \
  DATABASES="market_data order_engine api"
```

Default database list:

```text
hft market_data order_engine api analytics
```

Override the list:

```bash
make backup-databases DATABASES="market_data order_engine api"
```

Restore one database from the newest matching dump:

```bash
make restore-db DB=api
```

Restore one database from a specific dump:

```bash
make restore-db DB=api RESTORE_FILE=backups/api-20260613220606.dump
```

Restore the default application databases:

```bash
make restore-databases
```

Default restore database list:

```text
api market_data order_engine
```

Override the restore list:

```bash
make restore-databases RESTORE_DATABASES="market_data order_engine api"
```

Restore to a remote PostgreSQL host:

```bash
make restore-databases \
  POSTGRES_HOST=db.example.com \
  POSTGRES_USER=root \
  POSTGRES_PASSWORD='REPLACE_WITH_PASSWORD' \
  POSTGRES_SSLMODE=require
```

## How to Run Strategy

Strategies are configured through the dashboard/API:

1. Create or enable `indicator_configs`.
2. Create a `strategy_configs` row for the pair, user, order sizing, and risk settings.
3. Create enabled `strategy_rules` for entry/exit conditions.
4. Run `strategy-service`.

Local command:

```bash
go run . strategy-service
```

Compose command:

```bash
make up-service
```

Scale executor workers:

```bash
docker compose --profile infra --profile app up -d --scale strategy-service=3
```

### Configuration Checklist

- Ensure strategy symbols match canonical internal symbol mapping (`symbol_mappings.symbol`).
- Ensure `market_type` alignment (`spot` or `futures`) across subscriptions and mappings.
- Ensure every active `strategy_configs` row has non-empty `user_id`.
- Ensure each `strategy_configs.user_id` maps to configured exchange credentials.
- Set required API keys and NATS/database hosts in `config.yml`.
- Tune indicators in `indicator_configs` and rule conditions in `strategy_rules`.

## Test Order API

### HTTP

```bash
curl --request POST \
  --url http://localhost:9801/order-engine/v1/orders \
  --header 'Content-Type: application/json' \
  --data '{
    "api_key": "REPLACE_WITH_SECURE_API_KEY_A",
    "request_id": "e1580b74-1af6-47e4-9b90-354875f1f19z",
    "user_id": "2cb989ec-5aea-422a-bb74-d735ae5d7230",
    "order_id": "1d421596-2ff1-4798-b1b5-5336ed39122z",
    "exchange": "tokocrypto",
    "market_type": "spot",
    "symbol": "TKO_IDR",
    "type": "LIMIT",
    "side": "BUY",
    "price": "920",
    "quantity": "30",
    "source": "http-hit",
    "strategy_id": "d9b0ba9a-db44-4bb4-b862-a60d2e0299a6",
    "strategy_name": "TREND_FOLLOWING_ATR",
    "interval": "15m",
    "internal": "SOL_USDT.P",
    "need_notification": true,
    "is_paper_trading": true
  }'
```

### gRPC Payload Example

```json
{
  "request_id": "e1580b74-1af6-47e4-9b90-354875f1f19e",
  "user_id": "2cb989ec-5aea-422a-bb74-d735ae5d7230",
  "order_id": "1d421596-2ff1-4798-b1b5-5336ed391227",
  "exchange": "tokocrypto",
  "market_type": "spot",
  "symbol": "TKO_IDR",
  "type": "LIMIT",
  "side": "BUY",
  "price": "920",
  "quantity": "30",
  "source": "grpc-hit",
  "strategy_id": "d9b0ba9a-db44-4bb4-b862-a60d2e0299a6",
  "is_paper_trading": true
}
```

### Market Data gRPC Backfill Payload Example

```json
{
  "end_time": "1775725027000",
  "exchange": "tokocrypto",
  "interval": "15m",
  "market_type": "spot",
  "start_time": "1773046627000",
  "symbol": "TKO_IDR"
}
```

## Roadmap

- Build analytic service that consumes market data.
- Support HA deployment for all services.
- Improve multi-user credential management UX (dashboard/admin APIs).
- Support multiple exchanges.

## Support This Project

If this project helps you, please consider donating to support development and infrastructure costs.

- USDT (TRC20, TRON): `TWqgxX7SZHCnCsx4VJNWtJUgmmiwk78W6S`
- SOL (Solana Network): `BisxMDwGpFUmSJzUtHjHhDjzSxdJQ3Luc3TPVD98hypj`

Every donation helps maintain this project and accelerate new features.

## Contributing

Contributions are very welcome. If you want to improve this project, you are invited to open a PR.

### How to Contribute

1. Fork this repository.
2. Create a feature/fix branch.
3. Make focused changes with clear commit messages.
4. Run tests/checks for your changes.
5. Open a Pull Request with a short description of problem and solution.

### Contribution Ideas

- New exchange connectors.
- Better risk management controls.
- Performance profiling and latency optimization.
- Improved strategy evaluation and reporting.
