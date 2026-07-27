package middleware

import (
	"net/http"

	"go.uber.org/zap"
)

// Recovery recovers from panics in downstream handlers, logs the panic with
// a stack trace, and returns HTTP 500 instead of crashing the process.
func Recovery(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						zap.String("request_id", RequestIDFromContext(r.Context())),
						zap.Any("panic", rec),
						zap.Stack("stack"),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
