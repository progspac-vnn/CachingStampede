# Cache Stampede Lab

A production-style Go backend built to reproduce, analyse, and solve a
Cache Stampede (Thundering Herd) problem in a cache-aside architecture.
See [CLAUDE.md](./CLAUDE.md) for the full engineering guidelines,
architecture rules, and milestone roadmap.

## Status

- **Milestone 1 — Application foundation**: config, structured logging,
  PostgreSQL/Redis connection lifecycles, HTTP middleware, Prometheus
  metrics registration, `/health` and `/ready` endpoints, graceful
  shutdown. Done.
- **Milestone 2 — Persistence**: SQL migrations, seed data, repository
  layer, `Product` model. Done.

No caching, business logic, or product APIs exist yet — those begin at
Milestone 3.

## Prerequisites

- Go 1.22+ (the repo was developed against 1.26.5; anything the module's
  `go.mod` declares or newer works)
- Docker (for PostgreSQL and Redis)

## Running locally

```bash
cp .env.example .env
make docker-up      # start PostgreSQL and Redis
make migrate-up      # apply SQL migrations
make db-seed          # load sample product data
make run                # start the API on :8080
```

Tear down with `make docker-down`. Roll back the schema with
`make migrate-down` (reverts the most recently applied migration).

## Verifying it's alive

```bash
curl http://localhost:8080/health   # {"status":"ok"}
curl http://localhost:8080/ready    # {"status":"ok"} once Postgres and Redis are reachable
curl http://localhost:8080/metrics  # Prometheus exposition format
```

## Project layout

```
cmd/api        entrypoint: HTTP server
cmd/migrate    entrypoint: SQL migration runner (up/down)
internal/config     env-based configuration loading
internal/logger     zap-based structured logging
internal/db          PostgreSQL connection pool (pgx/v5)
internal/cache      Redis client (go-redis/v9)
internal/middleware  request id, logging, recovery, timeout
internal/metrics     Prometheus collectors and HTTP instrumentation
internal/model       domain types (Product)
internal/repository  storage access for domain types
migrations       versioned SQL schema migrations
scripts          seed data and other dev scripts
docker-compose.yml   local PostgreSQL + Redis
```
