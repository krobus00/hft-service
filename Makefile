ifneq (,$(wildcard .env))
include .env
export
endif

DOCKER_NAMESPACE ?= krobus00
VERSION ?= latest
HFT_VERSION ?= $(VERSION)
WEB_VERSION ?= $(VERSION)
STRATEGY_VERSION ?= $(VERSION)
NEXT_PUBLIC_API_BASE_URL ?= http://localhost:9804/api/v1

HFT_IMAGE ?= $(DOCKER_NAMESPACE)/hft-service:$(HFT_VERSION)
WEB_IMAGE ?= $(DOCKER_NAMESPACE)/krobot-web:$(WEB_VERSION)
STRATEGY_IMAGE ?= $(DOCKER_NAMESPACE)/python-strategy:$(STRATEGY_VERSION)
HFT_LOCAL_IMAGE ?= hft-service:local
WEB_LOCAL_IMAGE ?= krobot-web:local
STRATEGY_LOCAL_IMAGE ?= python-strategy:local

PROFILES ?= infra app web strategy
COMPOSE_PROFILE_FLAGS := $(foreach profile,$(PROFILES),--profile=$(profile))
BACKUP_COMPOSE_FILE ?=
BACKUP_COMPOSE_FILE_FLAGS := $(if $(BACKUP_COMPOSE_FILE),-f $(BACKUP_COMPOSE_FILE),)
BACKUP_COMPOSE := docker compose $(BACKUP_COMPOSE_FILE_FLAGS) $(COMPOSE_PROFILE_FLAGS)
BACKUP_POSTGRES_IMAGE ?= postgres:16-alpine

POSTGRES_USER ?= root
POSTGRES_PASSWORD ?= root
POSTGRES_HOST ?=
POSTGRES_PORT ?= 5432
POSTGRES_SSLMODE ?= prefer
REDIS_PASSWORD ?=
NATS_USER ?= hft
NATS_PASSWORD ?=
POSTGRES_SERVICE ?= postgresql
DATABASES ?= hft market_data order_engine api analytics
DB ?= hft
BACKUP_DIR ?= backups
TIMESTAMP ?= $(shell date +%Y%m%d%H%M%S)
RESTORE_DATABASES ?= api market_data order_engine
RESTORE_FILE ?=
RESTORE_CLEAN_FLAGS ?= --clean --if-exists
RESTORE_FLAGS ?= $(RESTORE_CLEAN_FLAGS) --no-owner --no-privileges

STRATEGY_FILE ?= krobot01
STRATEGY_FILES ?= $(filter-out standard_strategy_template,$(basename $(notdir $(wildcard strategy/*.py))))

.PHONY: build-service build-web build-strategy build-images build-local-images push-service push-web push-strategy push-images compose-pull up-service up-local-service down-service down-local-service compose-logs compose-local-logs compose-config compose-local-config run-strategy rerun-all-strategy backup-preflight backup-db backup-databases backup-all restore-db restore-databases restore-all

build-service:
	@docker build \
		-t $(HFT_IMAGE) \
		--target final \
		.

build-web:
	@docker build \
		-t $(WEB_IMAGE) \
		--build-arg NEXT_PUBLIC_API_BASE_URL=$(NEXT_PUBLIC_API_BASE_URL) \
		-f web/Dockerfile \
		web

build-strategy:
	@docker build \
		-t $(STRATEGY_IMAGE) \
		-f tools/python-strategy/Dockerfile \
		.

build-images: build-service build-web build-strategy

build-local-images:
	@$(MAKE) build-service HFT_IMAGE=$(HFT_LOCAL_IMAGE)
	@$(MAKE) build-web WEB_IMAGE=$(WEB_LOCAL_IMAGE)
	@$(MAKE) build-strategy STRATEGY_IMAGE=$(STRATEGY_LOCAL_IMAGE)

push-service:
	@docker push $(HFT_IMAGE)

push-web:
	@docker push $(WEB_IMAGE)

push-strategy:
	@docker push $(STRATEGY_IMAGE)

push-images: push-service push-web push-strategy

compose-pull:
	@HFT_VERSION=$(HFT_VERSION) WEB_VERSION=$(WEB_VERSION) STRATEGY_VERSION=$(STRATEGY_VERSION) REDIS_PASSWORD="$(REDIS_PASSWORD)" NATS_USER="$(NATS_USER)" NATS_PASSWORD="$(NATS_PASSWORD)" \
		docker compose $(COMPOSE_PROFILE_FLAGS) pull

up-service:
	@HFT_VERSION=$(HFT_VERSION) WEB_VERSION=$(WEB_VERSION) STRATEGY_VERSION=$(STRATEGY_VERSION) REDIS_PASSWORD="$(REDIS_PASSWORD)" NATS_USER="$(NATS_USER)" NATS_PASSWORD="$(NATS_PASSWORD)" \
		docker compose $(COMPOSE_PROFILE_FLAGS) up -d

up-local-service:
	@HFT_LOCAL_IMAGE=$(HFT_LOCAL_IMAGE) WEB_LOCAL_IMAGE=$(WEB_LOCAL_IMAGE) STRATEGY_LOCAL_IMAGE=$(STRATEGY_LOCAL_IMAGE) \
		docker compose -f compose-local.yaml $(COMPOSE_PROFILE_FLAGS) up -d

down-service:
	@docker compose $(COMPOSE_PROFILE_FLAGS) down

down-local-service:
	@docker compose -f compose-local.yaml $(COMPOSE_PROFILE_FLAGS) down

compose-logs:
	@docker compose $(COMPOSE_PROFILE_FLAGS) logs -f --tail=200

compose-local-logs:
	@docker compose -f compose-local.yaml $(COMPOSE_PROFILE_FLAGS) logs -f --tail=200

compose-config:
	@HFT_VERSION=$(HFT_VERSION) WEB_VERSION=$(WEB_VERSION) STRATEGY_VERSION=$(STRATEGY_VERSION) REDIS_PASSWORD="$(REDIS_PASSWORD)" NATS_USER="$(NATS_USER)" NATS_PASSWORD="$(NATS_PASSWORD)" \
		docker compose $(COMPOSE_PROFILE_FLAGS) config

compose-local-config:
	@HFT_LOCAL_IMAGE=$(HFT_LOCAL_IMAGE) WEB_LOCAL_IMAGE=$(WEB_LOCAL_IMAGE) STRATEGY_LOCAL_IMAGE=$(STRATEGY_LOCAL_IMAGE) \
		docker compose -f compose-local.yaml $(COMPOSE_PROFILE_FLAGS) config

run-strategy:
	@docker run --rm -d \
	  --name $(STRATEGY_FILE)-runner \
	  -v $(CURDIR)/strategy:/app \
	  -w /app \
	  $(STRATEGY_IMAGE) \
	  bash -c "python $(STRATEGY_FILE).py"

rerun-all-strategy:
	@for strategy in $(STRATEGY_FILES); do \
	  echo "Restarting $$strategy-runner"; \
	  docker rm -f $$strategy-runner >/dev/null 2>&1 || true; \
	  docker run --rm -d \
	    --name $$strategy-runner \
	    -v $(CURDIR)/strategy:/app \
	    -w /app \
	    $(STRATEGY_IMAGE) \
	    bash -c "python $$strategy.py"; \
	done

backup-preflight:
	@if [ -n "$(POSTGRES_HOST)" ]; then \
	  if ! docker run --rm \
	    -e PGSSLMODE="$(POSTGRES_SSLMODE)" \
	    $(BACKUP_POSTGRES_IMAGE) \
	    pg_isready -h "$(POSTGRES_HOST)" -p "$(POSTGRES_PORT)" -U "$(POSTGRES_USER)" >/dev/null; then \
	    echo 'Remote PostgreSQL is not reachable.' >&2; \
	    echo 'Check POSTGRES_HOST, POSTGRES_PORT, and POSTGRES_SSLMODE.' >&2; \
	    exit 1; \
	  fi; \
	elif ! $(BACKUP_COMPOSE) ps --status running --services | grep -qx '$(POSTGRES_SERVICE)'; then \
	  echo 'PostgreSQL compose service "$(POSTGRES_SERVICE)" is not running.' >&2; \
	  echo 'Start production services with "make up-service", or dev services with "docker compose -f compose-dev.yaml up -d postgresql".' >&2; \
	  echo 'For dev backup/restore, pass BACKUP_COMPOSE_FILE=compose-dev.yaml.' >&2; \
	  exit 1; \
	fi

backup-db: backup-preflight
	@mkdir -p $(BACKUP_DIR)
	@echo "Backing up database $(DB)"
	@out="$(BACKUP_DIR)/$(DB)-$(TIMESTAMP).dump"; \
	tmp="$$out.tmp"; \
	if [ -n "$(POSTGRES_HOST)" ]; then \
	  docker run --rm \
	    -e PGPASSWORD="$(POSTGRES_PASSWORD)" \
	    -e PGSSLMODE="$(POSTGRES_SSLMODE)" \
	    $(BACKUP_POSTGRES_IMAGE) \
	    pg_dump -h "$(POSTGRES_HOST)" -p "$(POSTGRES_PORT)" -U "$(POSTGRES_USER)" -d "$(DB)" -Fc \
	    > "$$tmp"; \
	else \
	  $(BACKUP_COMPOSE) exec -T \
	    -e PGPASSWORD="$(POSTGRES_PASSWORD)" \
	    $(POSTGRES_SERVICE) \
	    pg_dump -U $(POSTGRES_USER) -d $(DB) -Fc \
	    > "$$tmp"; \
	fi; \
	if [ $$? -eq 0 ]; then \
	  mv "$$tmp" "$$out"; \
	else \
	  rm -f "$$tmp"; \
	  exit 1; \
	fi
	@echo "$(BACKUP_DIR)/$(DB)-$(TIMESTAMP).dump"

backup-databases: backup-preflight
	@mkdir -p $(BACKUP_DIR)
	@for db in $(DATABASES); do \
	  out="$(BACKUP_DIR)/$$db-$(TIMESTAMP).dump"; \
	  tmp="$$out.tmp"; \
	  echo "Backing up database $$db"; \
	  if [ -n "$(POSTGRES_HOST)" ]; then \
	    docker run --rm \
	      -e PGPASSWORD="$(POSTGRES_PASSWORD)" \
	      -e PGSSLMODE="$(POSTGRES_SSLMODE)" \
	      $(BACKUP_POSTGRES_IMAGE) \
	      pg_dump -h "$(POSTGRES_HOST)" -p "$(POSTGRES_PORT)" -U "$(POSTGRES_USER)" -d $$db -Fc \
	      > "$$tmp"; \
	  else \
	    $(BACKUP_COMPOSE) exec -T \
	      -e PGPASSWORD="$(POSTGRES_PASSWORD)" \
	      $(POSTGRES_SERVICE) \
	      pg_dump -U $(POSTGRES_USER) -d $$db -Fc \
	      > "$$tmp"; \
	  fi; \
	  if [ $$? -eq 0 ]; then \
	    mv "$$tmp" "$$out"; \
	    echo "$$out"; \
	  else \
	    rm -f "$$tmp"; \
	    exit 1; \
	  fi; \
	done

backup-all: backup-databases

restore-db: backup-preflight
	@dump="$(RESTORE_FILE)"; \
	if [ -z "$$dump" ]; then \
	  dump="$$(ls -t "$(BACKUP_DIR)/$(DB)-"*.dump 2>/dev/null | head -n 1)"; \
	fi; \
	if [ -z "$$dump" ] || [ ! -f "$$dump" ]; then \
	  echo 'Backup file not found.' >&2; \
	  echo 'Set RESTORE_FILE=path/to/file.dump or ensure a matching $(BACKUP_DIR)/$(DB)-*.dump exists.' >&2; \
	  exit 1; \
	fi; \
	echo "Restoring database $(DB) from $$dump"; \
	if [ -n "$(POSTGRES_HOST)" ]; then \
	  docker run --rm -i \
	    -e PGPASSWORD="$(POSTGRES_PASSWORD)" \
	    -e PGSSLMODE="$(POSTGRES_SSLMODE)" \
	    $(BACKUP_POSTGRES_IMAGE) \
	    pg_restore -h "$(POSTGRES_HOST)" -p "$(POSTGRES_PORT)" -U "$(POSTGRES_USER)" -d "$(DB)" $(RESTORE_FLAGS) \
	    < "$$dump"; \
	else \
	  $(BACKUP_COMPOSE) exec -T \
	    -e PGPASSWORD="$(POSTGRES_PASSWORD)" \
	    $(POSTGRES_SERVICE) \
	    pg_restore -U $(POSTGRES_USER) -d $(DB) $(RESTORE_FLAGS) \
	    < "$$dump"; \
	fi

restore-databases: backup-preflight
	@for db in $(RESTORE_DATABASES); do \
	  dump="$$(ls -t "$(BACKUP_DIR)/$$db-"*.dump 2>/dev/null | head -n 1)"; \
	  if [ -z "$$dump" ] || [ ! -f "$$dump" ]; then \
	    echo "Backup file not found for database $$db." >&2; \
	    echo "Expected a matching $(BACKUP_DIR)/$$db-*.dump file." >&2; \
	    exit 1; \
	  fi; \
	  echo "Restoring database $$db from $$dump"; \
	  if [ -n "$(POSTGRES_HOST)" ]; then \
	    docker run --rm -i \
	      -e PGPASSWORD="$(POSTGRES_PASSWORD)" \
	      -e PGSSLMODE="$(POSTGRES_SSLMODE)" \
	      $(BACKUP_POSTGRES_IMAGE) \
	      pg_restore -h "$(POSTGRES_HOST)" -p "$(POSTGRES_PORT)" -U "$(POSTGRES_USER)" -d "$$db" $(RESTORE_FLAGS) \
	      < "$$dump"; \
	  else \
	    $(BACKUP_COMPOSE) exec -T \
	      -e PGPASSWORD="$(POSTGRES_PASSWORD)" \
	      $(POSTGRES_SERVICE) \
	      pg_restore -U $(POSTGRES_USER) -d "$$db" $(RESTORE_FLAGS) \
	      < "$$dump"; \
	  fi; \
	done

restore-all: restore-databases
