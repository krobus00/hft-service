# Docker

Production Compose uses released Docker Hub images. It does not build application images during `up`.

Copy and edit the environment template first:

```bash
copy .env.example .env
```

Docker Compose reads `.env` automatically, and the Makefile includes `.env` when present. Keep image tags, ports, credentials, profiles, and backup defaults there.

## Images

Default image references:

- `krobus00/hft-service:${HFT_VERSION:-latest}`
- `krobus00/krobot-web:${WEB_VERSION:-latest}`
- `krobus00/python-strategy:${STRATEGY_VERSION:-latest}`

## Build And Publish

```bash
make build-images VERSION=1.0.0 NEXT_PUBLIC_API_BASE_URL=http://YOUR_HOST:9804/api/v1
make push-images VERSION=1.0.0
```

## Run Released Images

Set versions in `.env`:

```env
HFT_VERSION=1.0.0
WEB_VERSION=1.0.0
STRATEGY_VERSION=1.0.0
```

Then run:

```bash
make compose-pull
make up-service
```

Default profiles are `infra app web`. Add strategies with:

Set `PROFILES=infra app web strategy` in `.env`, then run `make up-service`.

## Run Local Images

Use [compose-local.yaml](compose-local.yaml) when you want Compose to use images that already exist on your machine instead of Docker Hub release tags.

Default local image names:

- `hft-service:local`
- `krobot-web:local`
- `python-strategy:local`

Build local images:

```bash
make build-local-images
```

Run local images:

```bash
make up-local-service
```

Inspect local Compose config:

```bash
make compose-local-config
```

Override local image names in `.env`:

```env
HFT_LOCAL_IMAGE=hft-service:local
WEB_LOCAL_IMAGE=krobot-web:local
STRATEGY_LOCAL_IMAGE=python-strategy:local
```

## Redis And NATS Passwords

Redis and NATS passwords are optional.

Set these in `.env`:

```env
REDIS_PASSWORD=REPLACE_WITH_REDIS_PASSWORD
NATS_USER=hft
NATS_PASSWORD=REPLACE_WITH_NATS_PASSWORD
```

If those passwords are set, update `config.yml` and `strategy/config.yml` URLs:

```yaml
nats_jetstream:
  url: "nats://hft:REPLACE_WITH_NATS_PASSWORD@nats:4222"

redis:
  api:
    cache_dsn: "redis://:REPLACE_WITH_REDIS_PASSWORD@redis:6379/2"
```

Strategy config examples:

```yaml
global:
  nats_url: nats://hft:REPLACE_WITH_NATS_PASSWORD@nats:4222
  redis_url: redis://:REPLACE_WITH_REDIS_PASSWORD@redis:6379/0
```

## Backups

```bash
make backup-db DB=api
make backup-databases
```

Backup files are written under `backups/` and ignored by git.

## Dev Tools

Generate Go protobuf files with the dev tools profile:

```bash
docker compose -f compose-dev.yaml --profile tools run --rm --build protobuf
```
