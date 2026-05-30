STRATEGY_FILE ?= krobot01
STRATEGY_FILES ?= $(filter-out standard_strategy_template,$(basename $(notdir $(wildcard strategy/*.py))))

.PHONY: run-strategy rerun-all-strategy
run-strategy:
	@docker run --rm -d \
	  --name $(STRATEGY_FILE)-runner \
	  -v $(CURDIR)/strategy:/app \
	  -w /app \
	  python-strategy:latest \
	  bash -c "python $(STRATEGY_FILE).py"

rerun-all-strategy:
	@for strategy in $(STRATEGY_FILES); do \
	  echo "Restarting $$strategy-runner"; \
	  docker rm -f $$strategy-runner >/dev/null 2>&1 || true; \
	  docker run --rm -d \
	    --name $$strategy-runner \
	    -v $(CURDIR)/strategy:/app \
	    -w /app \
	    python-strategy:latest \
	    bash -c "python $$strategy.py"; \
	done

build-strategy:
	@docker build \
		-t python-strategy:latest \
		-f tools/python-strategy/Dockerfile \
		. --no-cache

up-service:
	@docker compose --profile=infra --profile=app up -d --build

build-and-run:
	$(MAKE) build-strategy
	$(MAKE) up-service
	$(MAKE) run-strategy
