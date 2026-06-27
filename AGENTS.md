# Agent Guide: hft-service

@/home/krobus/.codex/RTK.md

Read this file before exploring the repository. It is the project map and change checklist; do not scan every file by default. Start with the files named for the task, then follow symbols with `rtk rg`.

## Working rules

- Preserve the user's working tree. Check `rtk git status --short` before editing and do not rewrite, revert, or format unrelated changes.
- Prefix shell commands with `rtk` as required by the imported RTK instructions.
- Never commit `config.yml`, `.env`, credentials, tokens, Discord URLs, database dumps, or private exchange/account data. Edit the `*.example` templates when configuration shape changes.
- Keep changes focused. Follow existing direct constructors and concrete types; do not add speculative interfaces, factories, packages, or dependencies.
- Trading, authentication, RBAC, migrations, and money calculations are high-risk. Preserve validation and error handling, use `decimal.Decimal` in Go money paths, and add/run a focused test.
- Treat `README.md`, `docs/API_SERVICE.md`, config examples, and Compose files as runtime contracts. Update only the documents affected by the change.
- Root `AGENT.md` is an untracked/user-owned file as of this guide's creation. Do not assume it duplicates or replaces this file, and do not modify it unless explicitly requested.

## Project in one page

This is an event-driven crypto trading system with three runtimes:

- Go 1.25 services: exchange market data, persistence, order execution/reconciliation, notifications, migrations, and the dashboard API.
- Go strategy executor: consumes enriched indicator candles, evaluates database-configured rules, and publishes order requests.
- Next.js 15/React 19 dashboard: authentication, RBAC-aware navigation, CRUD resources, manual orders, backfills, and reports.

Infrastructure is PostgreSQL 16, NATS JetStream, and Redis. There are three application databases with separate migration histories:

- `market_data`: klines, subscriptions, symbol mappings, and strategy configs.
- `order_engine`: order histories, trades/reports, and latest price references.
- `api`: dashboard users, refresh tokens, roles, permissions, pages, and settings.

Primary event flow:

1. `market-data-gateway` loads subscriptions/mappings and receives exchange websocket klines.
2. It normalizes symbols and publishes `KLINE.<EXCHANGE>.<SYMBOL>.<INTERVAL>`.
3. `market-data-worker` stores klines in `market_data`; strategies consume the same stream.
4. A strategy publishes an `entity.OrderRequestEvent`-compatible JSON envelope to `order_engine.place_order`.
5. `order-engine-gateway` resolves exchange symbols/credentials, places or simulates the order, stores history, and publishes notifications/position-close events.
6. `order-engine-worker` reconciles open orders and maintains price references.
7. `notification-service` consumes `order_engine.notification_alert` and calls Discord.
8. `api` reads all three databases, publishes manual orders, and calls market-data gRPC for backfills.

Authoritative architecture details live near the top of `README.md`. API endpoint semantics live in `docs/API_SERVICE.md`; exchange behavior lives in `internal/service/exchange/README.md`.

## Repository map: open only what the task needs

| Area | Start here | Then inspect |
|---|---|---|
| CLI/service startup | `main.go`, `cmd/<service>.go` | matching `internal/bootstrap/<service>*.go` |
| Configuration | `internal/config/config.go` | `config.yml.example`, Compose environment/volumes |
| Market data | `internal/service/exchange/exchange.go`, `exchange_common.go` | exchange implementation, `internal/service/kline/`, market-data repository/entity |
| Orders | `internal/service/orderengine/order_engine.service.go` | exchange implementation, order entity/repository, gateway handlers |
| Order reconciliation/reports | `internal/service/orderengine/order_history_sync.service.go` | `internal/repository/order_history.repository.go`, order-engine migrations |
| Dashboard API | `internal/handler/api/http/api.http.go` | `internal/service/api/`, relevant repository/entity, `docs/API_SERVICE.md` |
| Auth/RBAC | `internal/service/api/auth.service.go`, `internal/constant/api.go` | API auth repository, handler middleware, API migrations, web RBAC |
| Pagination/filtering | `internal/api/pagination.go` | entity `GetColumn`/`DefaultSort`/`SearchableFields`, repository helpers |
| Strategy rules | `internal/service/strategy/strategy.service.go` | strategy rule entity/repository, dashboard resource metadata |
| Dashboard resource | `web/src/lib/resources.ts` | `api-client.ts`, `types/api.ts`, generic resource components |
| Custom dashboard panel | relevant `web/src/components/organisms/*-panel.tsx` | `dashboard-app.tsx`, API client/types |
| Styling/UI primitive | `web/src/app/globals.css`, relevant component | existing `web/src/components/ui/`; reuse installed primitives |
| Database schema | newest files in `migration/postgresql/<database>/` | owning entity/repository/service only |
| Protobuf/gRPC | `pb/**/*.proto` | handler/client use sites, `tools/protobuf/Dockerfile` |
| Deployment | `Makefile`, requested Compose file | Dockerfile and `.env.example` |

Important naming oddity: `internal/bootstrap/market_data_worker.boostrap.go` has the historical misspelling `boostrap`. Do not create a second correctly spelled duplicate just to tidy it.

## Go architecture and conventions

The dependency direction is intentionally simple:

`cmd -> bootstrap -> handler/service/repository/infrastructure -> entity/config/constant/util`

- `cmd/` declares Cobra commands and flags. The root command loads config before service startup.
- `internal/bootstrap/` is the composition root. Add database/NATS/Redis connections and graceful shutdown cleanup here, not in business methods.
- `internal/handler/` translates HTTP/gRPC inputs and outputs. HTTP uses `net/http` `ServeMux`, not an external router.
- `internal/service/` owns workflows and event behavior.
- `internal/repository/` owns SQL and pagination. SQL uses `sqlx`, PostgreSQL placeholders, and Squirrel where useful.
- `internal/entity/` owns shared persistence/event types and normalization enums.
- `internal/infrastructure/` owns database, Redis, JetStream, and HTTP construction/health checks.
- `internal/api/` owns dashboard response, token, and pagination helpers.
- `internal/constant/` owns subjects, environments, response codes, and permission names.

Use existing constructor wiring rather than package globals, except established global config/exchange registry patterns. Pass `context.Context` through I/O. Wrap errors with useful operation context and preserve sentinel errors where handlers branch on `errors.Is`.

API responses must use `internal/api.WriteSuccess` / `WriteError` and the existing envelope (`message`, `code`, `data`, optional validation errors). Register a new API route in `Handler.Register`, enforce its HTTP method in the handler, and apply `withAuth` or `withPermission` as appropriate.

Pagination is allowlist-based. When adding a queryable entity field, update the entity's column map and searchable/default-sort definitions, then ensure repository SQL supports it. Never interpolate arbitrary client field names into SQL.

## Cross-service contracts and invariants

These fields form a pair identity and usually must remain consistent across subscription, mapping, strategy config, events, orders, and filters:

`exchange + market_type + canonical symbol + interval`

- Canonical symbols look like `BTC_USDT` and are stored in `symbol_mappings.symbol` and `kline_subscriptions.symbol`.
- `kline_symbol` is the exchange websocket form (for example `btcusdt`).
- `order_symbol` is the exchange order API form (for example `BTCUSDT`).
- `market_type` normalizes to `spot` when omitted; Binance supports spot/futures, Tokocrypto is currently spot.
- `user_id` selects `exchanges.<exchange>.accounts.<user_id>` credentials, with top-level exchange credentials as fallback.
- Strategy DB rows—not strategy YAML—own pair routing, user, source, position side, paper mode, order sizing, notification choice, and risk controls.
- `strategy_configs.strategy` is the strategy name used in Go rule executor orders and reports.

NATS contracts are defined in `internal/constant/kline.go` and used by Go services:

- Stream `KLINE`, subjects `KLINE.>` and `KLINE.<EXCHANGE>.<SYMBOL>.<INTERVAL>`.
- Stream `order_engine`, subject `order_engine.place_order` for orders and `order_engine.notification_alert` for alerts.
- Stream `strategy_control`, subject `strategy_control.position_closed` for external position closure.
- Event JSON uses an outer `{ "data": ... }` envelope. Keep Go `entity` JSON tags aligned.
- JetStream queue/durable names affect delivery semantics. Do not casually rename them or change retention policies.

Order semantics are centralized in `internal/entity/order_engine.go`:

- Spot accepts `BUY`/`SELL`; futures normalizes buy/long to `LONG` and sell/short to `SHORT`.
- `trade_condition`, `exit_type`, `entry_order_id`, `strategy_id`, paper/live mode, and position side affect reconciliation and reports. Trace their complete path before changing them.
- Do not use `float64` for Go prices, quantities, fees, or PnL. Use `shopspring/decimal` and preserve string serialization at boundaries.

## Database and migration rules

Migrations are Goose SQL files under `migration/postgresql/{api,market_data,order_engine}`. The CLI is:

```bash
rtk go run . migrate --databaseName=<api|market_data|order_engine>
```

Additional migration flags are declared in `cmd/migrate.go`; inspect that file rather than guessing syntax.

- Add a new timestamped migration; do not edit an already-applied migration unless explicitly told it has never shipped.
- Include both `-- +goose Up` and a safe `-- +goose Down`, with statement blocks where needed.
- Use the database directory that owns the table. Cross-database joins are not available; the API opens separate connections and combines results in Go.
- Keep entity `db` tags, repository scans/queries, API DTOs, strategy SQL, and docs/config examples synchronized with schema changes.
- RBAC changes normally require all of: permission constant, API migration seed/update, route middleware, dashboard resource permission, and API docs.
- Validate destructive or backfill migrations against realistic data volume and lock behavior; never run reset/down against user data without explicit authorization.

## Go strategy executor

`internal/service/strategy/strategy.service.go` owns database-configured strategy execution. It consumes `KLINE_INDICATOR`, loads enabled `strategy_configs` and `strategy_rules`, evaluates rule JSON, and publishes `order_engine.place_order`.

- Configure indicators in `indicator_configs`.
- Configure strategy pair/order/risk/user settings in `strategy_configs`.
- Configure entry/exit conditions in `strategy_rules`.
- Use NATS queue subscription semantics for horizontal scaling; multiple `strategy-service` instances share work.

## Web dashboard

The dashboard is a strict TypeScript Next.js app using App Router, Tailwind, and existing Radix/shadcn-style primitives. The `@/*` alias maps to `web/src/*`.

- `web/src/lib/resources.ts` is the primary declarative registry for generic dashboard resources: key, endpoint path, permissions, columns, filters, enums, defaults, and sample bodies.
- `web/src/components/organisms/resource-panel.tsx` and molecule components implement generic CRUD. Prefer resource metadata over a new page/component when it fits.
- Specialized reports/backfills/overview panels live in `components/organisms/` and are selected in `dashboard-app.tsx`.
- `web/src/lib/api-client.ts` owns the API envelope, authenticated requests, refresh behavior, and request deduplication.
- `web/src/lib/auth-store.ts` owns browser session storage; `auth-api.ts` owns auth calls; `rbac.ts` owns permission checks.
- Backend dashboard pages are also database-managed. A new navigable resource may require `ResourceKey`, resource metadata, API route/permission, and an `api` migration for `dashboard_pages`/role permissions.
- Reuse existing UI components and design tokens. Do not add a component library or state manager for a single feature.
- `NEXT_PUBLIC_API_BASE_URL` is a build-time public value and defaults to `http://localhost:9804/api/v1`.

## Protobuf and generated code

Source files are:

- `pb/market_data/market_data.proto`
- `pb/order_engine/order.proto`
- `pb/order_engine/order_engine.proto`

The matching `*.pb.go` and `*_grpc.pb.go` files are generated. Do not hand-edit them. Regenerate with the repository tool:

```bash
rtk docker build -t hft-protobuf -f tools/protobuf/Dockerfile .
rtk docker run --rm -v "$PWD:/workspace" -w /workspace hft-protobuf
```

After a proto change, update every Go handler/client conversion and event/API contract affected, then run Go formatting/tests.

## Local runtime and deployment

Templates are `config.yml.example` and `.env.example`. Runtime copies are intentionally untracked.

- Local Go processes use `localhost` for PostgreSQL/NATS/Redis/gRPC.
- Containers use Compose service names: `postgresql`, `nats`, `redis`, and `market-data-gateway`.
- `compose-dev.yaml` is infrastructure-oriented local development.
- `compose-local.yaml` runs locally built application images.
- `compose.yaml` runs versioned published images and profiles (`infra app web strategy` by default in the Makefile).
- Root `Dockerfile` builds the Go binary, and `web/Dockerfile` builds Next.js.
- Default ports: order-engine gRPC `9800`, order-engine HTTP `9801`, market-data gRPC `9802`, API HTTP `9804`, dashboard `3000`, strategy monitors `19011+`.

Do not start the full stack for a code-only change. If Compose changes, validate rendering with the matching `make compose-config` or `make compose-local-config`; note that it may require local environment values.

## Validation: smallest relevant set

Run focused checks first, then the broader check if shared contracts changed.

Go:

```bash
rtk gofmt -w <changed-go-files>
rtk go test ./<changed-package>
rtk go test ./...
rtk go build ./...
```

Web:

```bash
cd web
rtk npm run lint
rtk npm run typecheck
rtk npm run build
```

Use `npm ci` only when dependencies must be installed from the existing lockfile. Do not modify `package-lock.json` unless dependencies intentionally changed.

Migration/Compose/protobuf changes also require the domain-specific validation described above. Tests that require PostgreSQL, NATS, Redis, exchange access, or credentials must be reported as not run if those dependencies are unavailable; do not fake successful integration coverage.

## Change checklists

### New or changed API resource

1. Migration/entity/repository query and allowlisted pagination fields.
2. Service method and HTTP handler route/method/validation.
3. Auth or permission middleware and permission seed if needed.
4. API response envelope/status behavior and focused Go test.
5. Web `ResourceKey`, resource registry/client/types/panel as needed.
6. `docs/API_SERVICE.md` and config examples if the public contract changed.

### New strategy or strategy config field

1. Decide ownership: algorithm/runtime YAML versus pair/order/risk DB row.
2. Update model/loader/framework only if the shared runtime needs it.
3. Implement the strategy from the template and add one focused unittest.
4. Add example config, README description, Docker/Compose service/profile only if it must ship as a standalone runner.
5. Verify strategy key, pair identity, monitor port, subjects, and Redis state.

### New order/event field

1. Trace producer to consumer before editing.
2. Update Go entity JSON tags and event payloads.
3. Update DB migration/entity/repository when persisted.
4. Update protobuf only if the gRPC boundary uses the field, then regenerate.
5. Update API/web/report DTOs only where exposed.
6. Add a focused contract/behavior test and run `go test ./...`.

### New exchange behavior

1. Extend existing `entity.Exchange` behavior only when the common interface requires it.
2. Implement mapping, websocket parsing, REST signing/order conversion, and market-type rules in the exchange package.
3. Register it in `internal/bootstrap/exchange_init.go` and expose config/examples/enums.
4. Verify canonical/kline/order symbol directions and credential lookup.
5. Add deterministic tests around parsing/signing/normalization; do not call a live exchange in unit tests.

## Definition of done

- The smallest relevant implementation is complete and follows existing architecture.
- Cross-runtime/schema/event contracts are synchronized.
- Generated files were regenerated, not edited manually.
- Formatting and relevant tests/builds pass, or unavailable external prerequisites are stated precisely.
- No secrets or unrelated user changes entered the diff.
- Final handoff names changed files, checks run, and any concrete remaining risk.
