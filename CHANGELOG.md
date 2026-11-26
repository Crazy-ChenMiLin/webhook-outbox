# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). Before 1.0, minor versions may change the
schema or the configuration; such changes are marked **BREAKING**.

## [Unreleased]

### Fixed

- Honor `Retry-After` on webhook responses with status 429 or 503, up to the
  longest configured retry pause, and record the value on the attempt.

## [0.1.0] - 2026-11-25

### Added

- Tables `webhook_endpoints`, `webhook_messages`, `webhook_deliveries`,
  `webhook_attempts` for PostgreSQL 14 or later, as `schema/postgresql.sql` and as
  `webhook-outbox-relay migrate`. See [docs/schema.md](docs/schema.md).
- `webhook-outbox-relay endpoints add|list|secret|disable|enable`.
- `webhook-outbox-relay run`: dispatches messages to endpoints by customer and type,
  signs requests per Standard Webhooks, retries on a schedule (8 attempts over about 28
  hours by default) and records every attempt. Several relays can share a database.
  See [docs/relay.md](docs/relay.md).
- The relay disables an endpoint that answers 410 Gone, or whose attempts have all
  failed for `RELAY_DISABLE_AFTER` (5 days by default).
- The relay refuses to connect to loopback, private, link-local and other non-public
  addresses, checked after DNS. `RELAY_ALLOW_PRIVATE_NETWORKS=true` turns this off.
- `webhook-outbox-relay replay`: send messages again, or catch an endpoint up after an
  outage with `-endpoint ID -since TIME`.
- `/metrics` (attempts, durations, backlog and lags), `/healthz`, `/readyz` on
  `RELAY_HTTP_ADDR`, and `webhook-outbox-relay healthcheck` for the image.
- PHP: `WebhookOutbox::send()` queues a webhook in the current PDO transaction, with
  a Standard Webhooks body and a ULID message id; `Webhook::verify()` and `sign()` for
  receivers; `Testing\InMemorySender`. See [docs/php.md](docs/php.md).
- An example in `examples/shop`: a shop that queues webhooks in its transaction, the
  relay, and a receiver with endpoints that answer slowly, fail, hang or are gone,
  checked end to end in CI.

[Unreleased]: https://github.com/IanFoxDev/webhook-outbox/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/IanFoxDev/webhook-outbox/releases/tag/v0.1.0
