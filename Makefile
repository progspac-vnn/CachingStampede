# The system-installed `go` is 1.18; this module requires a newer toolchain
# (go-redis/v9, pgx/v5, client_golang). Prefer a local go1.26.5 SDK if
# present, otherwise fall back to whatever `go` is on PATH.
GO := $(shell if [ -x "$(HOME)/sdk/go1.26.5/bin/go" ]; then echo "$(HOME)/sdk/go1.26.5/bin/go"; else echo "go"; fi)

# k6 isn't packaged for this system either; prefer a local install if present.
K6 := $(shell if [ -x "$(HOME)/sdk/k6-2.1.0/k6" ]; then echo "$(HOME)/sdk/k6-2.1.0/k6"; else echo "k6"; fi)

-include .env
POSTGRES_USER ?= postgres
POSTGRES_DB ?= cachestampede

.PHONY: run tidy fmt docker-up docker-down migrate-up migrate-down db-seed load-test

run:
	$(GO) run ./cmd/api

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down

migrate-up:
	$(GO) run ./cmd/migrate up

migrate-down:
	$(GO) run ./cmd/migrate down

db-seed:
	docker compose exec -T postgres psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) < scripts/seed.sql

load-test:
	$(K6) run load/stampede.js
