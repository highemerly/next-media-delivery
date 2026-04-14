package handler

import (
	"net/http"
	"testing"
)

func TestCheckETagMatch(t *testing.T) {
	tests := []struct {
		name     string
		inm      string // If-None-Match header value
		etag     string // server-side opaque value
		want     bool
	}{
		// No header → no match.
		{name: "empty header", inm: "", etag: "abc123", want: false},

		// Wildcard.
		{name: "wildcard", inm: "*", etag: "abc123", want: true},
		{name: "wildcard with spaces", inm: "  *  ", etag: "abc123", want: true},

		// Strong ETag (RFC 7232 §2.3).
		{name: "strong match", inm: `"abc123"`, etag: "abc123", want: true},
		{name: "strong no match", inm: `"xyz"`, etag: "abc123", want: false},

		// Weak ETag.
		{name: "weak match", inm: `W/"abc123"`, etag: "abc123", want: true},
		{name: "weak no match", inm: `W/"xyz"`, etag: "abc123", want: false},

		// Comma-separated list — any match is sufficient.
		{name: "list first matches", inm: `"abc123", "other"`, etag: "abc123", want: true},
		{name: "list second matches", inm: `"other", "abc123"`, etag: "abc123", want: true},
		{name: "list mixed weak/strong", inm: `W/"other", "abc123"`, etag: "abc123", want: true},
		{name: "list none match", inm: `"foo", "bar"`, etag: "abc123", want: false},

		// Nginx-style unquoted value — treated as malformed, should not match.
		{name: "unquoted value", inm: "abc123", etag: "abc123", want: false},

		// Malformed entries are skipped; valid ones in the list still work.
		{name: "mixed malformed and valid", inm: `bad, "abc123"`, etag: "abc123", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			if tt.inm != "" {
				req.Header.Set("If-None-Match", tt.inm)
			}
			got := checkETagMatch(req, tt.etag)
			if got != tt.want {
				t.Errorf("checkETagMatch(inm=%q, etag=%q) = %v, want %v",
					tt.inm, tt.etag, got, tt.want)
			}
		})
	}
}
