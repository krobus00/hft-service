STRATEGY_FILE ?= krobot01

.PHONY: run-strategy
run-strategy:
	docker run --rm -d \
	  --name $(STRATEGY_FILE)-runner \
	  -v $(CURDIR)/strategy:/app \
	  -w /app \
	  python-strategy:latest \
	  bash -c "python $(STRATEGY_FILE).py"
