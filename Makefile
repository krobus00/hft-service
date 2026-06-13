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

PROFILES ?= infra app web
COMPOSE_PROFILE_FLAGS := $(foreach profile,$(PROFILES),--profile=$(profile))

POSTGRES_USER ?= root
POSTGRES_PASSWORD ?= root
REDIS_PASSWORD ?=
NATS_USER ?= hft
NATS_PASSWORD ?=
POSTGRES_SERVICE ?= postgresql
DATABASES ?= hft market_data order_engine api analytics
DB ?= hft
BACKUP_DIR ?= backups
TIMESTAMP ?= $(shell date +%Y%m%d%H%M%S)

STRATEGY_FILE ?= krobot01
STRATEGY_FILES ?= $(filter-out standard_strategy_template,$(basename $(notdir $(wildcard strategy/*.py))))

.PHONY: build-service build-web build-strategy build-images build-local-images push-service push-web push-strategy push-images compose-pull up-service up-local-service down-service down-local-service compose-logs compose-local-logs compose-config compose-local-config run-strategy rerun-all-strategy backup-db backup-databases backup-all

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

backup-db:
	@mkdir -p $(BACKUP_DIR)
	@echo "Backing up database $(DB)"
	@docker compose exec -T \
		-e PGPASSWORD=$(POSTGRES_PASSWORD) \
		$(POSTGRES_SERVICE) \
		pg_dump -U $(POSTGRES_USER) -d $(DB) -Fc \
		> $(BACKUP_DIR)/$(DB)-$(TIMESTAMP).dump
	@echo "$(BACKUP_DIR)/$(DB)-$(TIMESTAMP).dump"

backup-databases:
	@mkdir -p $(BACKUP_DIR)
	@for db in $(DATABASES); do \
	  echo "Backing up database $$db"; \
	  docker compose exec -T \
	    -e PGPASSWORD=$(POSTGRES_PASSWORD) \
	    $(POSTGRES_SERVICE) \
	    pg_dump -U $(POSTGRES_USER) -d $$db -Fc \
	    > $(BACKUP_DIR)/$$db-$(TIMESTAMP).dump; \
	done

backup-all: backup-databases
