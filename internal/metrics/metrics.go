// Package metrics registers the Prometheus collectors used to observe the
// application: the HTTP layer, cache-aside hit/miss behavior, and
// PostgreSQL connection pool saturation.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the Prometheus collectors registered for this application
// instance. It owns a private registry rather than relying on the global
// default registerer, so multiple instances never collide.
type Metrics struct {
	registry            *prometheus.Registry
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
	cacheHitsTotal      prometheus.Counter
	cacheMissesTotal    prometheus.Counter
	cacheStaleTotal     prometheus.Counter
	lockContendedTotal  prometheus.Counter
	dbFetchesTotal      prometheus.Counter
}

// New creates a Metrics instance with a private registry and registers the
// standard Go/process collectors alongside the application's HTTP and cache
// metrics.
func New() *Metrics {
	registry := prometheus.NewRegistry()

	httpRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Latency of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	cacheHitsTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total number of cache-aside lookups served from Redis.",
	})

	cacheMissesTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "Total number of cache-aside lookups that missed Redis and fell back to PostgreSQL.",
	})

	cacheStaleTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_stale_served_total",
		Help: "Total number of lookups served a stale (soft-expired) cached value while a background refresh ran.",
	})

	lockContendedTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_lock_contended_total",
		Help: "Total number of times a request found the distributed refill lock already held by another request.",
	})

	dbFetchesTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "product_db_fetches_total",
		Help: "Total number of times the product repository actually queried PostgreSQL, after coalescing and locking.",
	})

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequestsTotal,
		httpRequestDuration,
		cacheHitsTotal,
		cacheMissesTotal,
		cacheStaleTotal,
		lockContendedTotal,
		dbFetchesTotal,
	)

	return &Metrics{
		registry:            registry,
		httpRequestsTotal:   httpRequestsTotal,
		httpRequestDuration: httpRequestDuration,
		cacheHitsTotal:      cacheHitsTotal,
		cacheMissesTotal:    cacheMissesTotal,
		cacheStaleTotal:     cacheStaleTotal,
		lockContendedTotal:  lockContendedTotal,
		dbFetchesTotal:      dbFetchesTotal,
	}
}

// CacheHit records a cache-aside lookup served from Redis.
func (m *Metrics) CacheHit() {
	m.cacheHitsTotal.Inc()
}

// CacheMiss records a cache-aside lookup that missed Redis.
func (m *Metrics) CacheMiss() {
	m.cacheMissesTotal.Inc()
}

// CacheStaleServed records a lookup served a stale cached value under
// stale-while-revalidate.
func (m *Metrics) CacheStaleServed() {
	m.cacheStaleTotal.Inc()
}

// LockContended records a request finding the distributed refill lock
// already held.
func (m *Metrics) LockContended() {
	m.lockContendedTotal.Inc()
}

// DBFetch records an actual PostgreSQL query made to refill the cache,
// after in-process and cross-process coalescing.
func (m *Metrics) DBFetch() {
	m.dbFetchesTotal.Inc()
}

// RegisterPostgresPool registers gauges that read the given pool's live
// connection stats on every scrape: connections currently acquired, idle,
// and the configured maximum. This makes pool saturation (e.g. under a
// cache stampede) directly observable.
func (m *Metrics) RegisterPostgresPool(pool *pgxpool.Pool) {
	m.registry.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "postgres_pool_acquired_conns",
			Help: "Number of connections currently checked out of the PostgreSQL pool.",
		}, func() float64 { return float64(pool.Stat().AcquiredConns()) }),

		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "postgres_pool_idle_conns",
			Help: "Number of idle connections in the PostgreSQL pool.",
		}, func() float64 { return float64(pool.Stat().IdleConns()) }),

		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "postgres_pool_max_conns",
			Help: "Maximum number of connections allowed in the PostgreSQL pool.",
		}, func() float64 { return float64(pool.Stat().MaxConns()) }),
	)
}

// Handler returns the HTTP handler that exposes the registry in the
// Prometheus exposition format, intended to be mounted at /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Middleware records request count and latency for every HTTP request that
// passes through it, labelled by method, route pattern, and status code.
func (m *Metrics) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			status := strconv.Itoa(rec.status)
			duration := time.Since(start).Seconds()

			m.httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
			m.httpRequestDuration.WithLabelValues(r.Method, r.URL.Path, status).Observe(duration)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}
