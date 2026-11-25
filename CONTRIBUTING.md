# Contributing

## Running locally

You need Docker, PHP 8.3+ with `pdo_pgsql`, and Composer. Go is optional: without a
local toolchain the Makefile runs Go in a container.

```bash
composer install
make postgres-up    # PostgreSQL 18 on 55434
make check          # PHPUnit, PHPStan max, go vet, go test -race, golangci-lint
make relay-image    # the relay image, webhook-outbox-relay:dev
make postgres-down
```

Tests that need PostgreSQL run only when it is configured: `WEBHOOK_OUTBOX_PG_DSN` for
PHP, `WEBHOOK_OUTBOX_TEST_DATABASE_URL` for Go. The Makefile sets both for the
container above. Without them the tests are skipped, not failed, so check the output
if a run looks fast. CI fails on skipped tests.

The example runs end to end in compose:

```bash
cd examples/shop
docker compose up -d --build --wait
docker compose run --rm --build e2e
docker compose down -v
```

## Where things are

| Path | What |
|---|---|
| `src/` | The PHP package: `WebhookOutbox` (queue), `Webhook` (verify, sign), `Testing\InMemorySender` |
| `relay/internal/schema/migrations/` | The tables. `make schema` puts them together into `schema/postgresql.sql`; a test fails when that file is stale |
| `relay/internal/relay` | The round: dispatch, claim, send, record |
| `relay/internal/netguard` | The check that refuses private addresses |
| `relay/internal/endpoint`, `replay`, `metrics`, `admin` | The commands and the HTTP port |
| `examples/shop` | The shop, the receiver with failing endpoints, the end-to-end test |
| `docs/adr/` | Decisions and why |

## Before you write code

Open an issue first for anything bigger than a fix. Queueing only from the
application's transaction, at-least-once delivery and the refusal of private
addresses are deliberate; a change to them needs a test that shows what still holds.

A change to the tables goes into a new migration, never into a released one, and
then `make schema`.

## Pull requests

- One change per pull request, with a test that fails without it.
- `make check` passes.
- Behavior, settings or metrics changed: update `docs/` and `CHANGELOG.md` under
  `[Unreleased]`.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
  `feat:`, `fix:`, `docs:`, `test:`, `ci:`, `chore:`.
