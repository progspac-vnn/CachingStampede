package middleware

import "context"

type contextKey string

const requestIDContextKey contextKey = "request_id"

// RequestIDFromContext returns the request ID stored in ctx by RequestID
// middleware, or an empty string if none is present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey).(string)
	return id
}
