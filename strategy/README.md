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

make run-strategy STRATEGY_FILE=trend_following_atr

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
