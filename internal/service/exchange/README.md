# Exchange Implementation Guide

This package contains exchange adapters. An adapter should keep exchange-specific behavior local while reusing the shared helpers for common service behavior.

## Required Interface

Every exchange must implement `entity.Exchange`:

- `HandleKlineData(ctx, message)` parses websocket kline messages and publishes a `MarketKlineEvent`.
- `SubscribeKlineData(ctx, subscriptions)` opens websocket subscriptions and supports symbol/subscription resync.
- `BackfillMarketKlines(ctx, req)` fetches historical klines and stores them.
- `PlaceOrder(ctx, order)` submits an order and maps the exchange response into `OrderHistory`.
- `SyncOrderHistory(ctx, orderHistory)` refreshes an order from the exchange.

Register the implementation in its initializer with `RegisterExchange`.

## File Layout

Prefer small files by responsibility when an exchange grows:

- `<exchange>.exchange.go`: struct, initializer, public interface methods, endpoint selection.
- `<exchange>.kline.go`: websocket payload structs, kline parsing, subscription URL/payload handling.
- `<exchange>.order.go`: order placement, order detail sync, request signing, response mapping.
- `<exchange>.precision.go`: exchange-info lookup and quantity/price normalization.
- `<exchange>.types.go`: private exchange DTOs and code conversion helpers.

Shared logic belongs in a generic helper only when at least two exchanges need the same behavior.

## Adapter Checklist

1. Add the exchange name to `internal/entity/exchange.go`.
2. Create a config entry in `config.ExchangeConfig`.
3. Load symbol mappings in the initializer and call `persistSymbolMapping`.
4. Normalize credentials with `loadExchangeAccounts` and resolve them with `credentialsForUser`.
5. Use `recvWindowFromEnv` for signed REST APIs that support a receive window.
6. Use `subscribeKlineInsertEvents` and `insertKlineEvent` for JetStream kline persistence.
7. Use `exchangeKlineResyncDeps` for symbol mapping, websocket resync, and order-symbol translation.
8. Keep exchange codes at the boundary. Convert order sides, order types, and statuses into `entity` values before returning from the adapter.
9. Quantize quantity and price before submitting orders. Reject values that become zero after normalization.
10. Return errors with enough context: exchange name, endpoint intent, status code, exchange code/message, and parse body when useful.

## Conventions

- Normalize symbols with `strings.ToUpper(strings.TrimSpace(...))` before cache or mapping lookup.
- Do not store exchange symbols in domain records. Convert back to the internal symbol before publishing or returning history.
- Keep API response structs private unless another package truly needs them.
- Avoid logging secrets, signed payloads, or headers. Debug signing logs must stay behind an explicit env flag.
- Use `context.Context` for every network and repository call.
- Do not silently ignore malformed numeric fields from exchange responses.

## Adding Tests

For each adapter, add table tests for:

- order side/type/status conversion;
- client order ID normalization;
- websocket kline payload parsing;
- REST success and rejection mapping using `httptest.Server`;
- precision quantization edge cases.

Tests should not call real exchange endpoints.
