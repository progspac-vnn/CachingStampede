package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/progspac-vnn/CachingStampede/internal/cache"
	"github.com/progspac-vnn/CachingStampede/internal/db"
	"github.com/progspac-vnn/CachingStampede/internal/metrics"
	"github.com/progspac-vnn/CachingStampede/internal/middleware"
)

// dependencies bundles the infrastructure required to serve routes.
type dependencies struct {
	postgres *db.Postgres
	redis    *cache.Redis
	metrics  *metrics.Metrics
	timeout  time.Duration
}

// newRouter wires middleware and routes onto a chi.Mux. Only infrastructure
// routes (/health, /ready, /metrics) are registered here — product routes
// are out of scope for this milestone.
func newRouter(deps dependencies, log *zap.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID())
	r.Use(middleware.Logging(log))
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Timeout(deps.timeout))
	r.Use(deps.metrics.Middleware())

	r.Get("/health", handleHealth)
	r.Get("/ready", handleReady(deps))
	r.Handle("/metrics", deps.metrics.Handler())

	return r
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleReady(deps dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		pgErr := deps.postgres.Health(ctx)
		redisErr := deps.redis.Health(ctx)

		if pgErr != nil || redisErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status":   "unavailable",
				"postgres": statusString(pgErr),
				"redis":    statusString(redisErr),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func statusString(err error) string {
	if err != nil {
		return "unavailable"
	}
	return "ok"
}

func writeJSON(w http.ResponseWriter, status int, body map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
