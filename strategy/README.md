# Python Strategy

Python strategy runtime for market data driven algorithmic trading.

## Strategy Description

This module contains executable strategy files plus a shared framework for:
- Consuming candle streams from NATS.
- Running warmup + live decision loops.
- Publishing orders to the order engine.
- Applying centralized risk controls from configuration.
- Auto-resyncing `strategy_configs` rows while the process is running.
- Persisting strategy state in Redis so restarts can continue from the last known indicator/position state.
- Exposing a lightweight HTTP monitor endpoint.

Main strategy design goals:
- Keep strategy-specific logic inside each strategy file (for example `krobot01.py`, `ai.py`).
- Reuse common execution plumbing from `core/`.
- Standardize signal, order, and risk behavior across strategies.

### Built-in Strategies

#### `krobot01.py` (EMA + VWAP + MACD Cross)

- Signal style: Rule-based trend/momentum entry.
- Long entry: Price above EMA and VWAP, with MACD bullish cross.
- Short entry: Price below EMA and VWAP, with MACD bearish cross.
- Exit logic: Take profit, stop loss, trailing stop, plus optional intrabar risk exits.
- Risk behavior: Cooldown after trades, stronger cooldown after stop loss, and pause after stop-loss streak.
- Extra guard: Re-entry lock on the same side until opposite MACD cross appears.

#### `krobot02.py` (EMA + VWAP + Volume Confirmation)

- Signal style: Rule-based trend + participation filter.
- Long entry: Price above EMA and VWAP, and current volume exceeds rolling average by multiplier.
- Short entry: Price below EMA and VWAP, and current volume exceeds rolling average by multiplier.
- Exit logic: Take profit, stop loss, trailing stop.
- Risk behavior: Cooldown, stop-loss cooldown, and pause after configured stop-loss streak.
- Primary use case: Capture moves with stronger volume confirmation to reduce weak breakouts.

#### `krobot03.py` (EMA Cross + Bollinger/ATR + RSI/Stochastic)

- Signal style: EMA ribbon cross entries with momentum and volatility confirmation.
- Trend trigger: EMA 21 crossing EMA 50 (golden/death cross behavior).
- Volatility filter: Bollinger Bands location checks and optional ATR minimum-percent filter.
- Momentum filter: RSI thresholds by default, or Stochastic K/D mode via config toggle.
- Exit logic: Opposite EMA cross, take profit, stop loss, trailing stop, and max-hold timeout.

#### `ai.py` (AI Hybrid Adaptive Strategy)

- Signal style: LLM-driven decisioning with structured market payload.
- Input features: Recent kline snapshots, EMA/VWAP/MACD/ATR/RSI context, trend state, prior trade conditions, and current position risk state.
- Decisions: AI returns `BUY`, `SELL`, `HOLD`, or `EXIT` with confidence and reason.
- Adaptive risk: AI can suggest dynamic take-profit, stop-loss, and trailing-stop values when confidence is high.
- Position management: Supports TP ladder/roll behavior (hold winners and step TP) when trend and confidence remain supportive.
- Safety controls: Intrabar risk guard blocks LLM calls during intrabar exit checks; hard risk exits still run locally.

#### `supertrend.py` (Pure Supertrend Trend-Following Strategy)

- Signal style: Indicator-only trend following using Supertrend (ATR + multiplier).
- Trade behavior: Uptrend signal sends LONG (`BUY`), downtrend signal sends SHORT (`SELL`).
- Position behavior: Always-in-position model; on opposite signal, strategy reverses direction immediately (no intentional flat gap state).
- Risk behavior: No local TP/SL/trailing-stop logic. Strategy fully trusts Supertrend direction changes.
- Runtime standard: Pair routing, user mapping, and risk-config resolution are handled by `strategy_configs` + shared `core/framework.py` (no per-strategy `kline_subject`/`queue_name` settings).

#### `micro_grid.py` (Micro Grid)

- Buys when price falls one configured grid step below the latest anchor.
- Sells when price rises one configured grid step above the latest anchor and an open grid buy exists.
- Grid buys are tracked as entries and closed FIFO so positions and PnL use the standard order reports.
- Uses `python-micro-grid` as its `strategy_configs` key.

#### `capital_guard.py` (Guarded Trend Scalper)

- Signal style: Conservative trend/momentum entry for liquid, short-interval markets.
- Long entry: EMA 21 > EMA 55 > EMA 200, close above rolling VWAP, MACD bullish cross, RSI in a bounded bullish range, volume confirmation, and ATR/range filters passing.
- Short entry: EMA 21 < EMA 55 < EMA 200, close below rolling VWAP, MACD bearish cross, RSI in a bounded bearish range, volume confirmation, and ATR/range filters passing.
- Exit logic: Take profit, stop loss, trailing stop, protective VWAP/EMA/MACD exits, and max-hold timeout.
- Risk behavior: Cooldown after trades, stronger cooldown after stop loss, pause after stop-loss streak, and same-side re-entry lock until the opposite MACD cross appears.
- Primary use case: Prefer fewer, higher-confirmation trades over constant exposure. It is not a profitability guarantee.

## Quick Start

### 1) Build image

Build from `tools/python-strategy/Dockerfile`:

```bash
docker build \
  -t python-strategy:latest \
  -f tools/python-strategy/Dockerfile \
  .
```

### 2) Run with Makefile (recommended)

From repository root:

```bash
make run-strategy
```

Override strategy file:

```bash
make run-strategy STRATEGY_FILE=krobot01
```

Example running the guarded strategy:

```bash
make run-strategy STRATEGY_FILE=capital_guard
```

The Make target uses:
- `Makefile` variable: `STRATEGY_FILE` (default: `krobot01`)
- Volume mount: `$(CURDIR)/strategy:/app`

### 3) Run directly with Docker

Example running `krobot01.py`:

```bash
STRATEGY_FILE=krobot01.py
docker run --rm \
  --name strategy-runner \
  -v $(pwd)/strategy:/app \
  -w /app \
  python-strategy:latest \
  bash -c "python ${STRATEGY_FILE}"
```

## Required Runtime Configuration

Before running any strategy, configure both files and DB rows:

- Root config: `config.yml`
- Strategy config: `strategy/config.yml`
- Database table: `strategy_configs`

### 1) Root config (`config.yml`)

Configure exchange credentials. For multi-user routing, use per-user accounts:

```yaml
exchanges:
  binance:
    accounts:
      minimax-01:
        api_key: "..."
        api_secret: "..."
  tokocrypto:
    accounts:
      paper-1:
        api_key: "..."
        api_secret: "..."
```

### 2) Strategy config (`strategy/config.yml`)

Each strategy section should only define strategy-owned settings, such as indicator windows, thresholds, warmup limits, and its monitor port. Do not define `user_id`, `strategy_id`, `source`, `position_side`, order sizing, paper-trading flags, or risk controls in YAML. Those belong in `strategy_configs`. The order publish subject is shared by every strategy and is hardcoded in the framework as `order_engine.place_order`.

### 3) Strategy configs (`strategy_configs`)

Strategies now use pair-level config from this table (not from YAML), including risk controls:

```sql
INSERT INTO strategy_configs
(strategy, exchange, market_type, symbol, interval, user_id, position_side, source,
 need_notification, is_paper_trading, order_type, order_qty, limit_slippage_pct,
 cooldown_bars, sl_cooldown_bars, max_consecutive_stop_losses, sl_pause_bars,
 take_profit_pct, stop_loss_pct, trailing_stop_pct, trailing_stop_trigger_pct, max_hold_bars, max_positions, enable_intrabar_risk_exit)
VALUES
('python-ai-minimax-m2-7-hybrid', 'binance', 'futures', 'BTC_USDT', '1m', 'minimax-01',
 'BOTH', 'python-ai-minimax-m2-7',
 true, false, 'MARKET', 10, 0.02,
 2, 3, 2, 10,
 0.25, 0.15, 0.12, 0.20, 24, 1, true);
```

Rules:

- `strategy` must match the strategy key used by the Python runner, for example `python-krobot01-ema200-vwap-macd`.
- For `capital_guard.py`, use `python-capital-guard`.
- For `micro_grid.py`, use `python-micro-grid`.
- `symbol/interval/exchange/market_type` must match active market-data rows.
- `user_id` must match a configured exchange account key.
- `position_side` and `source` are stored in DB rows so one strategy process can route different pairs correctly.
- Orders are always published to `order_engine.place_order`; this is framework behavior, not strategy config.

## Project Layout

Shared reusable framework in `core/`:
- `core/framework.py`: warmup flow, live feed handling, NATS reconnect callbacks, DB config resync, Redis state persistence, monitor endpoints, order publish, shared buy/sell helpers.
- `core/indicators.py`: reusable indicators (EMA, RollingVWAP, MACD).
- `core/models.py`: shared candle/signal/runtime dataclasses.
- `core/common.py`: config and utility helpers.

Use `standard_strategy_template.py` as the starting point for new strategies.

## Risk Controls

Risk controls are now sourced from table `strategy_configs` per strategy+pair row.

Resolution order used by each strategy runtime:
1. `strategy_configs.<key>` row value
2. Strategy section value (optional local fallback)
3. `global.<key>` (optional local fallback)
4. Hardcoded strategy default

### Control Definitions

- `cooldown_bars`: Minimum bars to wait after entry/exit before allowing new entries.
- `sl_cooldown_bars`: Cooldown used specifically after `STOP_LOSS` exits. Effective cooldown after SL is `max(cooldown_bars, sl_cooldown_bars)`.
- `max_consecutive_stop_losses`: Number of consecutive SL events allowed before triggering a hard pause.
- `sl_pause_bars`: Hard pause duration (bars) after SL streak threshold is reached.
- `take_profit_pct`: Take-profit threshold in percent from entry.
- `stop_loss_pct`: Stop-loss threshold in percent from entry.
- `trailing_stop_pct`: Trailing callback distance in percent from best favorable price since entry.
- `trailing_stop_trigger_pct`: Trailing activation threshold in percent from entry before callback tracking is armed.
- `max_hold_bars`: Maximum bars a position can be held before forced timeout exit.
- `max_positions`: Maximum concurrent positions allowed by strategy decision logic.
- `enable_intrabar_risk_exit`: Enables risk exits on price updates between closed candles.

### Tuning Tips

- If trades are too frequent after losses: increase `sl_cooldown_bars`, reduce `max_consecutive_stop_losses`, increase `sl_pause_bars`.
- If strategy exits too early: increase `stop_loss_pct` and/or `trailing_stop_pct` carefully.
- If strategy holds losers too long: decrease `stop_loss_pct`, and for AI strategy decrease `max_hold_bars`.

## Runtime Reliability

### NATS reconnect

The shared runner enables NATS reconnect through `RuntimeConfig`:

- `nats_allow_reconnect`
- `nats_max_reconnect_attempts`
- `nats_reconnect_time_wait_sec`
- `nats_connect_timeout_sec`
- `nats_ping_interval_sec`
- `nats_max_outstanding_pings`

Callbacks log disconnect/reconnect/error/closed events. If the connection is fully closed, the runner exits so the process supervisor can restart it.

### DB config resync

The runner reloads `strategy_configs` every `global.strategy_config_refresh_interval_sec` seconds. Added rows are warmed up and subscribed. Removed or changed rows are unsubscribed and re-added with the latest config.

Each active pair has its own in-memory queue and worker. NATS callbacks enqueue messages quickly, then the pair worker processes candles in order. Different pairs can run independently, while each individual pair stays sequential so indicator and position state remain consistent.

Postgres access uses an asyncpg pool:

```yaml
global:
  db_pool_min_size: 1
  db_pool_max_size: 4
  strategy_config_refresh_interval_sec: 10
```

### Redis state

Set `global.redis_url` to persist per-pair strategy state:

```yaml
global:
  redis_url: redis://localhost:6379/0
  redis_key_prefix: hft:strategy
  redis_state_ttl_sec: 86400
  redis_delete_removed_state: false
```

The runner stores indicator state, position/risk state, remembered entry price, and entry order ID after each processed message. On startup or config resync, Redis state is restored first; DB warmup is used when no Redis state exists for the pair.

### Monitor Endpoints

Enable the built-in HTTP monitor:

```yaml
global:
  # shared defaults only

krobot01:
  monitor_enabled: true
  monitor_host: 127.0.0.1
  monitor_port: 19011

krobot02:
  monitor_enabled: true
  monitor_host: 127.0.0.1
  monitor_port: 19012
```

Endpoints:

- `GET /health`: process, NATS, Redis, and resync health.
- `GET /metrics`: message, signal, order, reconnect, and config-resync counters.
- `GET /state`: full active pair, subscription, and JSON-safe strategy state snapshot.
- `GET /`: same as `/state`.
