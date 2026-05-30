# Python Strategy

Python strategy runtime for market data driven algorithmic trading.

## Strategy Description

This module contains executable strategy files plus a shared framework for:
- Consuming candle streams from NATS.
- Running warmup + live decision loops.
- Publishing orders to the order engine.
- Applying centralized risk controls from configuration.

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

#### `ai.py` (AI Hybrid Adaptive Strategy)

- Signal style: LLM-driven decisioning with structured market payload.
- Input features: Recent kline snapshots, EMA/VWAP/MACD/ATR/RSI context, trend state, prior trade conditions, and current position risk state.
- Decisions: AI returns `BUY`, `SELL`, `HOLD`, or `EXIT` with confidence and reason.
- Adaptive risk: AI can suggest dynamic take-profit, stop-loss, and trailing-stop values when confidence is high.
- Position management: Supports TP ladder/roll behavior (hold winners and step TP) when trend and confidence remain supportive.
- Safety controls: Intrabar risk guard blocks LLM calls during intrabar exit checks; hard risk exits still run locally.

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

## Project Layout

Shared reusable framework in `core/`:
- `core/framework.py`: warmup flow, live feed handling, NATS subscribe, order publish, shared buy/sell helpers.
- `core/indicators.py`: reusable indicators (EMA, RollingVWAP, MACD).
- `core/models.py`: shared candle/signal/runtime dataclasses.
- `core/common.py`: config and utility helpers.

Use `standard_strategy_template.py` as the starting point for new strategies.

## Risk Controls

Risk controls are centralized in `config.yml` under `global.risk_controls`.

Resolution order used by each strategy:
1. Strategy section value (for example `krobot01.cooldown_bars`)
2. `global.risk_controls.<key>`
3. `global.<key>`
4. Hardcoded strategy default

Example:

```yaml
global:
  risk_controls:
    cooldown_bars: 2
    sl_cooldown_bars: 3
    max_consecutive_stop_losses: 2
    sl_pause_bars: 10
    take_profit_pct: 0.40
    stop_loss_pct: 0.25
    trailing_stop_pct: 0.12
    max_hold_bars: 24
    max_positions: 1
    enable_intrabar_risk_exit: true
```

### Control Definitions

- `cooldown_bars`: Minimum bars to wait after entry/exit before allowing new entries.
- `sl_cooldown_bars`: Cooldown used specifically after `STOP_LOSS` exits. Effective cooldown after SL is `max(cooldown_bars, sl_cooldown_bars)`.
- `max_consecutive_stop_losses`: Number of consecutive SL events allowed before triggering a hard pause.
- `sl_pause_bars`: Hard pause duration (bars) after SL streak threshold is reached.
- `take_profit_pct`: Take-profit threshold in percent from entry.
- `stop_loss_pct`: Stop-loss threshold in percent from entry.
- `trailing_stop_pct`: Trailing-stop distance in percent from best favorable price since entry.
- `max_hold_bars`: Maximum bars a position can be held before forced timeout exit.
- `max_positions`: Maximum concurrent positions allowed by strategy decision logic.
- `enable_intrabar_risk_exit`: Enables risk exits on price updates between closed candles.

### Tuning Tips

- If trades are too frequent after losses: increase `sl_cooldown_bars`, reduce `max_consecutive_stop_losses`, increase `sl_pause_bars`.
- If strategy exits too early: increase `stop_loss_pct` and/or `trailing_stop_pct` carefully.
- If strategy holds losers too long: decrease `stop_loss_pct`, and for AI strategy decrease `max_hold_bars`.
