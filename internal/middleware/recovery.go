package middleware

import (
	"log/slog"
	"net/http"
)

// Recovery wraps h with a deferred panic recovery.
// On panic it logs the error and returns 500, preventing the process from crashing
// (particularly important for libvips panics).
func Recovery(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "panic", rec, "url", r.URL.String())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	})
}
