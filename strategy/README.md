# Python Strategy

## Build image from tools/python-strategy/Dockerfile
docker build \
  -t python-strategy:latest \
  -f tools/python-strategy/Dockerfile \
  .

## Run a specific strategy file (example: krobot01.py)
STRATEGY_FILE=krobot01.py
docker run --rm \
  --name strategy-runner \
  -v $(pwd)/strategy:/app \
  -w /app \
  python-strategy:latest \
  bash -c "python ${STRATEGY_FILE}"

## Run with Makefile (recommended)
From repository root:

make run-strategy

Override strategy file:

make run-strategy STRATEGY_FILE=krobot01

The Make target uses:
- `Makefile` variable: `STRATEGY_FILE` (default: `krobot01`)
- volume mount: `$(CURDIR)/strategy:/app`

## Standard strategy framework

The standard reusable framework is under `core/`:
- `core/framework.py`: shared warmup, live feed handler, NATS subscribe, order publish, shared buy/sell helpers.
- `core/indicators.py`: reusable indicators (EMA, RollingVWAP, MACD).
- `core/models.py`: shared candle/signal/runtime dataclasses.
- `core/common.py`: shared config and utility helpers.

Use `standard_strategy_template.py` as the starting point for all new strategies.

## Risk controls

Risk controls are centralized in `config.yml` under `global.risk_controls`.
Each strategy reads controls in this order:
1. strategy section value (for example `krobot01.cooldown_bars`)
2. `global.risk_controls.<key>`
3. `global.<key>`
4. hardcoded default in strategy code

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

### Control definitions

- `cooldown_bars`
  - Minimum bars to wait after entry/exit before allowing new entries.
  - Higher value reduces overtrading in choppy markets.

- `sl_cooldown_bars`
  - Cooldown used specifically after `STOP_LOSS` exits.
  - Effective cooldown after SL is `max(cooldown_bars, sl_cooldown_bars)`.

- `max_consecutive_stop_losses`
  - Number of consecutive SL events allowed before triggering a hard pause.

- `sl_pause_bars`
  - Hard pause duration (bars) after SL streak threshold is reached.
  - During pause, strategy will not evaluate new entries.

- `take_profit_pct`
  - Take-profit threshold in percent from entry.

- `stop_loss_pct`
  - Stop-loss threshold in percent from entry.

- `trailing_stop_pct`
  - Trailing-stop distance in percent from best favorable price since entry.

- `max_hold_bars`
  - Maximum bars a position can be held before forced timeout exit.

- `max_positions`
  - Maximum concurrent positions allowed by strategy decision logic.
  - Currently used by AI strategy risk payload / gating.

- `enable_intrabar_risk_exit`
  - Enables risk exits on price updates between closed candles.
  - This is consumed by the framework runtime.


### Tuning tips

- If trades are too frequent after losses:
  - Increase `sl_cooldown_bars`, reduce `max_consecutive_stop_losses`, increase `sl_pause_bars`.

- If strategy exits too early:
  - Increase `stop_loss_pct` and/or `trailing_stop_pct` carefully.

- If strategy holds losers too long:
  - Decrease `stop_loss_pct`, and for AI strategy decrease `max_hold_bars`.
