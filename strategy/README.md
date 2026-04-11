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
