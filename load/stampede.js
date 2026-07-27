// Reproduces a cache stampede (thundering herd) against the naive
// cache-aside GET /products/{id} endpoint.
//
// Scenario:
//   1. "prime"     — a single request populates the Redis cache entry.
//   2. wait for the cache TTL to expire.
//   3. "stampede"  — CONCURRENCY virtual users request the same product ID
//      at (as close to) the same instant, all racing past the now-expired
//      cache entry straight into PostgreSQL.
//
// The naive implementation has no request coalescing or locking (that's
// later milestones), so every one of those concurrent requests is expected
// to independently miss the cache and query the database.
//
// For this to reproduce deterministically within a short test run, start
// the API with a short cache TTL and pass the same value as TTL_SECONDS:
//
//   PRODUCT_CACHE_TTL=3s make run
//   docker compose exec redis redis-cli DEL product:1   # ensure a cold start
//   TTL_SECONDS=3 make load-test
//
// Environment variables:
//   BASE_URL     API base URL              (default: http://localhost:8080)
//   PRODUCT_ID   product ID to hammer      (default: 1)
//   TTL_SECONDS  must match PRODUCT_CACHE_TTL on the server (default: 3)
//   CONCURRENCY  number of simultaneous requests in the burst (default: 50)

import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const PRODUCT_ID = __ENV.PRODUCT_ID || '1';
const TTL_SECONDS = parseInt(__ENV.TTL_SECONDS || '3', 10);
const CONCURRENCY = parseInt(__ENV.CONCURRENCY || '50', 10);

export const stampedeFailures = new Counter('stampede_failures');

export const options = {
  scenarios: {
    prime: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 1,
      exec: 'prime',
      startTime: '0s',
    },
    stampede: {
      executor: 'shared-iterations',
      vus: CONCURRENCY,
      iterations: CONCURRENCY,
      maxDuration: '30s',
      exec: 'stampede',
      startTime: `${TTL_SECONDS + 1}s`,
    },
  },
};

export function prime() {
  const res = http.get(`${BASE_URL}/products/${PRODUCT_ID}`);
  check(res, { 'prime: status is 200': (r) => r.status === 200 });
}

export function stampede() {
  const res = http.get(`${BASE_URL}/products/${PRODUCT_ID}`);
  const ok = check(res, { 'stampede: status is 200': (r) => r.status === 200 });
  if (!ok) {
    stampedeFailures.add(1);
  }
}
