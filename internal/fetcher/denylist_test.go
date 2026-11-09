package fetcher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

type fakeDenylist struct {
	hosts map[string]bool
	ips   map[netip.Addr]bool
}

func (d fakeDenylist) DeniesHost(host string) bool { return d.hosts[host] }
func (d fakeDenylist) DeniesIP(ip netip.Addr) bool { return d.ips[ip.Unmap()] }

func newDenyTestFetcher(d Denylist) *HTTPFetcher {
	return New(Config{
		Timeout:             5 * time.Second,
		MaxRedirects:        3,
		MaxFileSize:         1 << 20,
		AllowedPrivateCIDRs: []string{"127.0.0.0/8"},
		Denylist:            d,
	})
}

// A redirect to a blacklisted host is refused before it is followed.
func TestFetchDeniesRedirectTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://blocked.example/a.png", http.StatusFound)
	}))
	defer srv.Close()

	f := newDenyTestFetcher(fakeDenylist{hosts: map[string]bool{"blocked.example": true}})
	_, err := f.Fetch(context.Background(), srv.URL)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("err = %v, want ErrDenied", err)
	}
}

// A blacklisted IP is refused at connect time, whatever name led to it.
func TestFetchDeniesDialedIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	defer srv.Close()

	f := newDenyTestFetcher(fakeDenylist{ips: map[netip.Addr]bool{netip.MustParseAddr("127.0.0.1"): true}})
	_, err := f.Fetch(context.Background(), srv.URL)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("err = %v, want ErrDenied", err)
	}

	// Same server, not denied.
	res, err := newDenyTestFetcher(fakeDenylist{}).Fetch(context.Background(), srv.URL)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("not denied: res = %+v, err = %v", res, err)
	}
}
