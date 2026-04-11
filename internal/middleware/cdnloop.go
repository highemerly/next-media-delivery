package middleware

import (
	"net/http"
	"strings"
)

const cdnLoopHeader = "CDN-Loop"

// CDNLoop detects request loops using RFC 8586 CDN-Loop header.
// If cdnName is empty, the middleware is a no-op.
func CDNLoop(cdnName string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cdnName != "" {
				loop := r.Header.Get(cdnLoopHeader)
				if loop != "" && containsID(loop, cdnName) {
					http.Error(w, "Loop Detected", http.StatusLoopDetected)
					return
				}
			}
			h.ServeHTTP(w, r)
		})
	}
}

// containsID reports whether the CDN-Loop header value contains the given cdn-id.
// Each cdn-info entry may have optional semicolon-separated parameters (RFC 8586),
// so only the cdn-id portion (before the first ";") is compared.
func containsID(headerVal, id string) bool {
	for _, part := range strings.Split(headerVal, ",") {
		entry := strings.TrimSpace(part)
		// Strip optional parameters (e.g. "; v=1.0").
		if i := strings.IndexByte(entry, ';'); i >= 0 {
			entry = strings.TrimSpace(entry[:i])
		}
		if entry == id {
			return true
		}
	}
	return false
}

// CDNLoopHeaderValue returns the value to set on outbound fetch requests.
// It appends our cdn-id to the existing value (or creates it).
func CDNLoopHeaderValue(existing, cdnName string) string {
	if existing == "" {
		return cdnName
	}
	return existing + ", " + cdnName
}
