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
- **Milestone 3 — Naive cache-aside**: `GET /products/{id}` — checks
  Redis first, falls back to PostgreSQL on a miss, then populates Redis
  before returning. Done.
- **Milestone 4 — Reproduce cache stampede**: a k6 load test
  (`load/stampede.js`) that forces the cache-aside endpoint into a
  thundering herd on TTL expiration. Done.
- **Milestone 5 — Observability**: Prometheus scraping the API, a
  provisioned Grafana dashboard (request rate, latency percentiles,
  cache hit ratio, hits vs misses, PostgreSQL pool saturation). Done.
- **Milestone 6 — Redis distributed lock**: a `SETNX` + Lua
  compare-and-delete lock in `internal/cache` coordinates cache refills
  across API processes. Done.
- **Milestone 7 — Go `singleflight`**: concurrent in-process requests for
  the same key are coalesced into a single database query. Done.
- **Milestone 8 — TTL jitter**: Redis's own (hard) expiry is the soft TTL
  plus the stale window plus random variance, so keys don't expire in
  lockstep. Done.
- **Milestone 9 — Stale-while-revalidate**: a soft-expired entry is still
  served immediately, with a coalesced background refresh. Done.
- **Milestone 10 — Benchmark comparison**: the same k6 stampede scenario
  from Milestone 4, rerun against the fully mitigated implementation. Done.

All ten milestones are implemented. See `docs/` for the full write-up.

## Prerequisites

- Go 1.22+ (the repo was developed against 1.26.5; anything the module's
  `go.mod` declares or newer works)
- Docker (for PostgreSQL, Redis, Prometheus, and Grafana)

## Running locally

```bash
cp .env.example .env
make docker-up      # start PostgreSQL, Redis, Prometheus, and Grafana
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
curl http://localhost:8080/products/1  # cache-aside product lookup
```

## Reproducing the cache stampede

`load/stampede.js` (k6) forces the cache-aside endpoint into a thundering
herd: it primes the cache with one request, waits for the TTL to expire,
then fires many concurrent requests at the same product ID.

To get a deterministic, fast reproduction, run the server with a short
TTL and make sure the target key starts cold:

```bash
PRODUCT_CACHE_TTL=3s PRODUCT_CACHE_STALE_TTL=0s PRODUCT_CACHE_JITTER=0 make run  # separate terminal
docker compose exec redis redis-cli DEL product:1     # ensure a cold start
TTL_SECONDS=3 make load-test
```

(Setting `STALE_TTL=0` and `JITTER=0` disables Milestones 8–9 so the key
hard-expires at exactly the TTL — useful for a clean, apples-to-apples
reproduction. Leave them at their defaults to see the full mitigated
behavior instead, including stale-while-revalidate.)

Interpreting the result:
- k6's summary reports request latency and error rate for the burst.
- The real proof is in the app's own logs and in two Prometheus counters:
  `cache_misses_total` (how many concurrent requests detected a miss) vs
  `product_db_fetches_total` (how many of those actually reached
  PostgreSQL). With Milestones 6–9 disabled, these two numbers are equal —
  every miss is its own query. With them enabled, misses can be dozens
  while fetches stay in the single digits.
- With the default PostgreSQL pool (`POSTGRES_MAX_CONNS=10`), an
  unmitigated burst larger than 10 will visibly queue for a connection,
  which is itself evidence of the resource contention a stampede causes
  downstream.

## Mitigating the stampede (Milestones 6–9)

- **Distributed lock** (`internal/cache/lock.go`) — `SETNX` acquires a
  per-key refill lock in Redis, released via a Lua compare-and-delete so a
  process can never release a lock it no longer holds. Coordinates cache
  refills *across* API processes.
- **`singleflight`** (`internal/service/product_coalescing.go`) —
  coalesces concurrent *in-process* requests for the same key into one
  call; every caller gets the same result.
- **TTL jitter** — Redis's own expiry is `TTL + STALE_TTL ± JITTER`, so
  keys cached around the same time don't all expire at the exact same
  instant.
- **Stale-while-revalidate** — a cache entry past its soft TTL (but
  within the stale window) is still served immediately; a background
  refresh is kicked off and coalesced through the same `singleflight`
  group.

Measured on this machine, rerunning the identical Milestone 4 scenario:

| Burst size | Concurrent misses detected | Actual DB queries | HTTP errors |
|---|---|---|---|
| 50 VUs (naive, M4) | 13 | 13 | 0% |
| 50 VUs (mitigated, M10) | 7 | 3 | 0% |
| 200 VUs (naive, M4) | 19 | 19 | 0% |
| 200 VUs (mitigated, M10) | 51 | 2 | 0% |

Isolating the distributed lock's own contribution (beyond what
`singleflight` alone provides): two independent API processes on
different ports, sharing the same Redis and PostgreSQL, each received one
simultaneous request for the same cold key. Only the process that won the
lock queried PostgreSQL; the other waited ~20ms and served the winner's
cached value — `product_db_fetches_total` on the loser's own `/metrics`
never moved.

## Observability

- Prometheus: http://localhost:9090 — scrapes the API's `/metrics` every
  5s. Try the query `postgres_pool_acquired_conns` while running the
  stampede load test to watch the pool saturate live.
- Grafana: http://localhost:3000 — the "Cache Stampede Lab" dashboard is
  provisioned automatically (no manual setup). Anonymous viewer access is
  enabled for local convenience; admin login defaults to
  `admin`/`admin` (override via `GRAFANA_ADMIN_USER`/`GRAFANA_ADMIN_PASSWORD`).
  Panels: HTTP request rate by status, latency p50/p95/p99, cache hit
  ratio, cache hits vs misses, PostgreSQL pool connections
  (acquired/idle/max), cache misses vs actual DB fetches (the Milestone
  6–9 payoff, visualized), and lock contention / stale-served requests.

## Project layout

```
cmd/api        entrypoint: HTTP server
cmd/migrate    entrypoint: SQL migration runner (up/down)
internal/config     env-based configuration loading
internal/logger     zap-based structured logging
internal/db          PostgreSQL connection pool (pgx/v5)
internal/cache      Redis client (go-redis/v9) + distributed lock
internal/middleware  request id, logging, recovery, timeout
internal/metrics     Prometheus collectors and HTTP instrumentation
internal/model       domain types (Product)
internal/repository  storage access for domain types
internal/service     business logic: cache-aside, singleflight, jitter, SWR
internal/handler     HTTP handlers (products)
migrations       versioned SQL schema migrations
scripts          seed data and other dev scripts
load             k6 load-testing scripts
docker            Prometheus scrape config, Grafana provisioning + dashboard
docker-compose.yml   local PostgreSQL + Redis + Prometheus + Grafana
docs             full engineering write-up(s), published as HTML
```
