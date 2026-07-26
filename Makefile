# The system-installed `go` is 1.18; this module requires a newer toolchain
# (go-redis/v9, pgx/v5, client_golang). Prefer a local go1.26.5 SDK if
# present, otherwise fall back to whatever `go` is on PATH.
GO := $(shell if [ -x "$(HOME)/sdk/go1.26.5/bin/go" ]; then echo "$(HOME)/sdk/go1.26.5/bin/go"; else echo "go"; fi)

.PHONY: run tidy fmt docker-up docker-down

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
