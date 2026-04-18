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

## Standard strategy framework

The standard reusable framework is under `core/`:
- `core/framework.py`: shared warmup, live feed handler, NATS subscribe, order publish, shared buy/sell helpers.
- `core/indicators.py`: reusable indicators (EMA, RollingVWAP, MACD).
- `core/models.py`: shared candle/signal/runtime dataclasses.
- `core/common.py`: shared config and utility helpers.

Use `standard_strategy_template.py` as the starting point for all new strategies.
