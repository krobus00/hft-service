# High Frequency Trading Service

## Setup Env

### Prerequisites
- Go `1.25+`
- Docker + Docker Compose
- PostgreSQL, Redis, and NATS are started from compose

### Configure application
1. Copy example config:
```bash
copy config.yml.example config.yml
```

2. Update these values in `config.yml`:
- `exchanges.tokocrypto.api_key`
- `exchanges.tokocrypto.api_secret`
- `api_keys[*].key` (for HTTP order auth)

3. Keep hostnames aligned with your runtime:
- **Run with Docker compose**: use `postgresql`, `redis`, `nats` hosts (already in `config.yml.example`)
- **Run app locally from terminal**: use `localhost` hosts (already in `config.yml`)

### Start infrastructure services
```bash
docker compose -f compose-dev.yaml up -d postgresql redis nats
```

### Create database
Connect to PostgreSQL, then run:
```
CREATE DATABASE market_data;
CREATE DATABASE order_engine;
```

### Run database migration
```bash
go run . migrate --databaseName=market_data
go run . migrate --databaseName=order_engine
```

### Seeding market_data database
```sql
INSERT INTO kline_subscriptions
(id, exchange, symbol, "interval", payload, created_at, updated_at)
VALUES('2675c22f-d94b-4ede-bb27-5b6c375a1089', 'tokocrypto', 'TKO_IDR', '1m', '{"id": 1, "method": "SUBSCRIBE", "params": ["tkoidr@kline_1m"]}'::jsonb, '2026-02-22 13:46:45.903', '2026-02-22 13:46:45.903');

INSERT INTO symbol_mappings
(id, exchange, symbol, kline_symbol, order_symbol, created_at, updated_at)
VALUES('4494faed-468c-46b5-b7ca-389419ad63ad', 'tokocrypto', 'TKOIDR', 'tkoidr', 'TKO_IDR', '2026-02-22 14:36:22.978', '2026-02-22 14:36:22.978');
```

### Run services
Open separate terminals:

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
go run . strategy
```

### Testing order
#### HTTP
```json
curl --request POST \
  --url http://localhost:9801/order-engine/v1/orders \
  --header 'Content-Type: application/json' \
  --header 'User-Agent: insomnia/12.3.1' \
  --data '{
	"api_key": "REPLACE_WITH_SECURE_API_KEY_A",
  "request_id": "e1580b74-1af6-47e4-9b90-354875f1f19z",
  "user_id": "2cb989ec-5aea-422a-bb74-d735ae5d7230",
  "order_id": "1d421596-2ff1-4798-b1b5-5336ed39122z",
  "exchange": "tokocrypto",
  "symbol": "TKO_IDR",
  "type": "LIMIT",
  "side": "BUY",
  "price": "920",
  "quantity": "30",
  "source": "http hit",
  "strategy_id": "d9b0ba9a-db44-4bb4-b862-a60d2e0299a6"
}'
```

#### gRPC
```json
{
  "request_id": "e1580b74-1af6-47e4-9b90-354875f1f19e",
  "user_id": "2cb989ec-5aea-422a-bb74-d735ae5d7230",
  "order_id": "1d421596-2ff1-4798-b1b5-5336ed391227",
  "exchange": "tokocrypto",
  "symbol": "TKO_IDR",
  "type": "LIMIT",
  "side": "BUY",
  "price": "920",
  "quantity": "30",
  "source": "grpc-hit",
  "strategy_id": "d9b0ba9a-db44-4bb4-b862-a60d2e0299a6"
}
```
# High Frequency Trading Service

## Setup Env

### Create database
```sql
CREATE DATABASE market_data;
CREATE DATABASE order_engine;
```

### Run database migration
```
go run . migrate --databaseName=market_data
go run . migrate --databaseName=order_engine
```

### Seeding market_data database
```sql
INSERT INTO kline_subscriptions
(id, exchange, symbol, "interval", payload, created_at, updated_at)
VALUES('2675c22f-d94b-4ede-bb27-5b6c375a1089', 'tokocrypto', 'TKO_IDR', '1m', '{"id": 1, "method": "SUBSCRIBE", "params": ["tkoidr@kline_1m"]}'::jsonb, '2026-02-22 13:46:45.903', '2026-02-22 13:46:45.903');


INSERT INTO symbol_mappings
(id, exchange, symbol, kline_symbol, order_symbol, created_at, updated_at)
VALUES('4494faed-468c-46b5-b7ca-389419ad63ad', 'tokocrypto', 'TKOIDR', 'tkoidr', 'TKO_IDR', '2026-02-22 14:36:22.978', '2026-02-22 14:36:22.978');
```

### Testing order
#### HTTP
```json
curl --request POST \
  --url http://localhost:9801/order-engine/v1/orders \
  --header 'Content-Type: application/json' \
  --header 'User-Agent: insomnia/12.3.1' \
  --data '{
	"api_key": "REPLACE_WITH_SECURE_API_KEY_A",
  "request_id": "e1580b74-1af6-47e4-9b90-354875f1f19z",
  "user_id": "2cb989ec-5aea-422a-bb74-d735ae5d7230",
  "order_id": "1d421596-2ff1-4798-b1b5-5336ed39122z",
  "exchange": "tokocrypto",
  "symbol": "TKO_IDR",
  "type": "LIMIT",
  "side": "BUY",
  "price": "920",
  "quantity": "30",
  "source": "http hit",
  "strategy_id": "d9b0ba9a-db44-4bb4-b862-a60d2e0299a6"
}'
```

#### gRPC
```json
{
  "request_id": "e1580b74-1af6-47e4-9b90-354875f1f19e",
  "user_id": "2cb989ec-5aea-422a-bb74-d735ae5d7230",
  "order_id": "1d421596-2ff1-4798-b1b5-5336ed391227",
  "exchange": "tokocrypto",
  "symbol": "TKO_IDR",
  "type": "LIMIT",
  "side": "BUY",
  "price": "920",
  "quantity": "30",
  "source": "grpc-hit",
  "strategy_id": "d9b0ba9a-db44-4bb4-b862-a60d2e0299a6"
}
```
