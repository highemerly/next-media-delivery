package fetcher

import (
	"testing"
	"time"
)

func TestTransportKeepsIdleConnsPerHost(t *testing.T) {
	f := New(Config{Timeout: time.Second, MaxRedirects: 1, MaxFileSize: 1})
	tr, ok := f.client.Transport.(*ssrfTransport)
	if !ok {
		t.Fatalf("transport is %T, want *ssrfTransport", f.client.Transport)
	}
	if got := tr.base.MaxIdleConnsPerHost; got != 32 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 32", got)
	}
}
