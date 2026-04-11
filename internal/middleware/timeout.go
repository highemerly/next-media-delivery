package middleware

import (
	"context"
	"net/http"
	"time"
)

// Timeout wraps each request with a context deadline of d.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
