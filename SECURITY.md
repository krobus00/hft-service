# Security Policy

## Supported Scope

Security reports are accepted for:

- Core services under internal/
- Public APIs and request validation
- Exchange credential handling and routing
- Strategy order publishing and event processing
- Docker/runtime configuration that can cause credential or data exposure

## Reporting a Vulnerability

Please report vulnerabilities privately.

Preferred process:

1. Open a private GitHub security advisory for this repository.
2. If advisory is unavailable, contact project maintainers privately.
3. Do not disclose details publicly until a fix is available.

## What to Include

- Affected component and file paths
- Reproduction steps or proof of concept
- Impact assessment
- Suggested remediation (if available)

## Response Targets

- Initial acknowledgment: within 72 hours
- Triage decision: within 7 days
- Fix timeline: depends on severity and complexity

## Severity Guidance

Higher priority:

- Credential leaks or bypasses
- Unauthorized order execution paths
- Auth/authz bypass in API endpoints
- Tampering with order routing/user mapping

## Safe Harbor

We appreciate good-faith security research that avoids service disruption, data destruction, and privacy violations.
