# Go targets use the local toolchain when available, otherwise the same commands run in Docker.
GO_IMAGE ?= golang:1.27
ifeq ($(shell command -v go 2>/dev/null),)
GO = docker run --rm -v $(CURDIR)/relay:/src -v webhook-outbox-gocache:/root/.cache -w /src $(GO_IMAGE)
else
GO = cd relay &&
endif
LINT_IMAGE ?= golangci/golangci-lint:v2.14.0
PG_IMAGE ?= postgres:18-alpine
# Tests that need PostgreSQL are skipped without it; make postgres-up starts this one.
export WEBHOOK_OUTBOX_TEST_DATABASE_URL ?= postgres://webhooks:webhooks@127.0.0.1:55434/webhooks
export WEBHOOK_OUTBOX_PG_DSN ?= pgsql:host=127.0.0.1;port=55434;dbname=webhooks;user=webhooks;password=webhooks

.PHONY: check php-test php-stan relay-test relay-vet relay-lint relay-build relay-image schema postgres-up postgres-down

check: php-test php-stan relay-vet relay-test relay-lint

vendor: composer.json
	composer update --no-progress
	touch vendor

php-test: vendor
	vendor/bin/phpunit

php-stan: vendor
	vendor/bin/phpstan analyse --no-progress

relay-test:
	$(GO) go test -race ./...

relay-vet:
	$(GO) go vet ./...

relay-lint:
	docker run --rm -v $(CURDIR)/relay:/src -w /src $(LINT_IMAGE) golangci-lint run

relay-build:
	$(GO) go build -o bin/webhook-outbox-relay ./cmd/webhook-outbox-relay

relay-image:
	docker build -t webhook-outbox-relay:dev relay

# schema/postgresql.sql is the migrations put together; a test fails when it is stale.
schema:
	{ cat relay/internal/schema/header.sql; for f in relay/internal/schema/migrations/*.sql; do printf '\n-- %s\n\n' "$$(basename $$f)"; cat "$$f"; done; } > schema/postgresql.sql

postgres-up:
	docker run -d --rm --name webhook-outbox-pg -p 55434:5432 \
		-e POSTGRES_USER=webhooks -e POSTGRES_PASSWORD=webhooks -e POSTGRES_DB=webhooks $(PG_IMAGE)
	# The init server listens on the socket only, so wait for TCP.
	until docker exec webhook-outbox-pg pg_isready -h 127.0.0.1 -U webhooks -q; do sleep 0.5; done

postgres-down:
	docker rm -f webhook-outbox-pg
