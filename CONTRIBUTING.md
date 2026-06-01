# Contributing Guide

Thank you for contributing.

## Ground Rules

- Follow the Code of Conduct in CODE_OF_CONDUCT.md.
- Keep pull requests focused and small when possible.
- Prefer clear commit messages and reproducible steps.
- Never commit secrets, API keys, or credentials.

## Development Setup

1. Copy config templates:

```bash
copy config.yml.example config.yml
copy strategy\config.yml.example strategy\config.yml
```

2. Start local infrastructure:

```bash
docker compose -f compose-dev.yaml up -d postgresql redis nats
```

3. Run migrations:

```bash
go run . migrate --databaseName=market_data
go run . migrate --databaseName=order_engine
```

## Build and Validation

Run these checks before opening a PR:

```bash
go build ./...
python -m py_compile strategy/core/framework.py strategy/ai.py strategy/krobot01.py strategy/krobot02.py
```

If you changed SQL or strategy routing logic, include test/verification notes in your PR.

## Branch and PR Workflow

1. Create a branch from main.
2. Make your changes.
3. Validate locally.
4. Open a PR using the template.

## What to Include in a PR

- Problem statement
- Solution summary
- Risk/impact notes
- Verification steps
- Config or migration changes

## Reporting Bugs

Use the bug issue template and include:

- Expected vs actual behavior
- Reproduction steps
- Logs/errors
- Environment details

## Security Issues

Do not open public issues for vulnerabilities or leaked credentials.
Please follow SECURITY.md.
