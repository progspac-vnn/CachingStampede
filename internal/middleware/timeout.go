package middleware

import (
	"net/http"
	"time"
)

// Timeout bounds request handling to the given duration. If the deadline is
// exceeded before the handler completes, callers relying on the request
// context (e.g. downstream DB/Redis calls) will observe context.DeadlineExceeded.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, "request timed out")
	}
}
