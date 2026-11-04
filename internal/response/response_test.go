package response

import (
	"testing"
	"time"
)

func TestServerTiming(t *testing.T) {
	tests := []struct {
		fetch, convert time.Duration
		want           string
	}{
		// L1 hit: neither fetched nor converted
		{0, 0, "nmdFetch;dur=0, nmdConvert;dur=0"},
		// A sub-millisecond fetch must not read as 0 ("not fetched")
		{423 * time.Microsecond, 0, "nmdFetch;dur=0.423, nmdConvert;dur=0"},
		{143 * time.Millisecond, 28*time.Millisecond + 100*time.Microsecond, "nmdFetch;dur=143, nmdConvert;dur=28.1"},
		// Sub-microsecond remainders are dropped
		{1500*time.Microsecond + 999*time.Nanosecond, 0, "nmdFetch;dur=1.5, nmdConvert;dur=0"},
	}
	for _, tt := range tests {
		if got := serverTiming(tt.fetch, tt.convert); got != tt.want {
			t.Errorf("serverTiming(%v, %v) = %q, want %q", tt.fetch, tt.convert, got, tt.want)
		}
	}
}
