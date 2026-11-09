package fetcher

import (
	"testing"
	"time"
)

// Through an HTTP(S)_PROXY the dialer would connect to the proxy, and the
// SSRF check in it would never see the origin's address.
func TestFetcherIgnoresEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example:3128")
	t.Setenv("HTTPS_PROXY", "http://proxy.example:3128")

	f := New(Config{Timeout: time.Second, MaxRedirects: 1, MaxFileSize: 1})
	tr, ok := f.client.Transport.(*ssrfTransport)
	if !ok {
		t.Fatalf("transport is %T, want *ssrfTransport", f.client.Transport)
	}
	if tr.base.Proxy != nil {
		t.Error("transport uses a proxy function; origin fetches must always dial directly")
	}
}
