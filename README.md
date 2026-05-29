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
- [Supported Exchanges](#supported-exchanges)
- [Market Type Support](#market-type-support)
- [Quick Start (Local)](#quick-start-local)
- [How to Run Strategy](#how-to-run-strategy)
- [Profit Results](#profit-results)
- [Test Order API](#test-order-api)
- [Roadmap](#roadmap)
- [Support This Project](#support-this-project)
- [Contributing](#contributing)

## How HFT Works in This Repository

This project follows an event-driven HFT loop:

1. Market data gateway subscribes to exchange websocket kline streams.
2. Incoming candles are normalized and published to NATS JetStream (`kline.data`).
3. Worker persists market data into the `market_data` database.
4. Strategy consumes closed candles and decides buy/sell/hold.
5. If action is needed, strategy publishes `order_engine.place_order`.
6. Order engine gateway executes the order to the exchange.
7. Order engine worker continuously reconciles order status until terminal state.
8. Notification service emits Discord alerts for important events.

In short: low-latency event ingestion + deterministic signal execution + robust post-trade reconciliation.

## System Architecture

### Event Flow

1. `market-data-gateway` consumes exchange kline data.
2. `market-data-gateway` publishes `kline.data` events.
3. `market-data-worker` consumes `kline.data` and stores records in `market_data` DB.
4. `strategy` consumes `kline.data` and decides: buy, sell, or no action.
5. If execution is required, `strategy` publishes `order_engine.place_order`.
6. `order-engine-gateway` consumes order events and executes to exchange.
7. `order-engine-gateway` stores initial order history in `order_engine` DB.
8. `order-engine-worker` polls tracked orders and fetches latest execution state from exchange.
9. `order-engine-worker` updates `order_histories` status in `order_engine` DB.
10. `notification-service` consumes notification events and sends Discord alerts.

### Diagram

```mermaid
flowchart TB
  EX[(Exchange)]
  NATS[(NATS JetStream)]
  MDG[market-data-gateway]
  MDW[market-data-worker]
  STRAT[strategy]
  OEG[order-engine-gateway]
  OEW[order-engine-worker]
  NS[notification-service]
  MDB[(market_data DB)]
  ODB[(order_engine DB)]
  DISCORD[(Discord)]

  EX -->|kline websocket| MDG
  MDG -->|publish kline.data| NATS
  NATS -->|consume kline.data| MDW
  NATS -->|consume kline.data| STRAT
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
- Publishes `kline.data` to JetStream.

### `market-data-worker`

- Consumes `kline.data` events.
- Persists market kline records using canonical internal symbol.
- Retries failed processing (bounded by config).

### `strategy`

- Consumes closed kline events.
- Runs strategy logic (buy/sell/hold).
- Publishes `order_engine.place_order` when execution is needed.

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

### `migrate`

- Runs DB migration for `market_data` and `order_engine`.

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

Update required values in `config.yml`:

- `exchanges.binance.api_key`
- `exchanges.binance.api_secret`
- `exchanges.tokocrypto.api_key`
- `exchanges.tokocrypto.api_secret`
- `api_keys[*].key`
- `notification.discord_webhook_url` (required when running `notification-service`)

Host mapping reminder:

- Run app locally (`go run`): use `localhost` hosts.
- Run app in Docker: use compose service names (`postgresql`, `redis`, `nats`).

### 3) Start infrastructure

```bash
docker compose -f compose-dev.yaml up -d postgresql redis nats
```

### 4) Create databases

Run in PostgreSQL:

```sql
CREATE DATABASE market_data;
CREATE DATABASE order_engine;
```

### 5) Run migrations

```bash
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

### 7) Run services (separate terminals)

```bash
go run . market-data-gateway
```

```bash
go run . market-data-worker
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
make build-strategy
make run-strategy
```

## How to Run Strategy

There are two common ways to run strategy in this repository.

### Option A: Run via Makefile (recommended)

This is the main runtime strategy flow used by this repository.

1. Start infrastructure and required services first:
  - `market-data-gateway`
  - `market-data-worker`
  - `order-engine-gateway`
  - `order-engine-worker`
2. Build strategy runner image:

```bash
make build-strategy
```

3. Run strategy:

```bash
make run-strategy
```

4. Verify the container is running:

```bash
docker ps --filter "name=-runner"
```

5. Verify logs show strategy consuming closed kline events and publishing `order_engine.place_order` when signals fire.

```bash
docker logs -f krobot01-runner
```

### Option B: Run a specific Python strategy file

For Python-based strategy scripts under `strategy/`:

```bash
make run-strategy STRATEGY_FILE=krobot01
```

This uses Docker image `python-strategy` and mounts local `strategy/` into `/app`.

### Configuration Checklist

- Ensure strategy symbols match canonical internal symbol mapping (`symbol_mappings.symbol`).
- Ensure `market_type` alignment (`spot` or `futures`) across subscriptions and mappings.
- Set required API keys and NATS/database hosts in `config.yml`.
- For Python strategy tuning, check risk controls in `strategy/config.yml` (`global.risk_controls`).

## Profit Results

Use this section to show transparent strategy performance over time.

### Performance Snapshot Template

| Strategy | Exchange | Market Type | Period | Net PnL | ROI | Max Drawdown | Win Rate |
| --- | --- | --- | --- | --- | --- | --- | --- |
| KRobot01 | Binance | Futures | 2026-01 to 2026-03 | +12.4% | +12.4% | -4.1% | 58% |
| KRobot02 | Tokocrypto | Spot | 2026-01 to 2026-03 | +7.2% | +7.2% | -2.8% | 54% |

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
- Support multiple user credentials.
- Support multiple exchanges.
- Dashboard

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
