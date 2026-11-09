package fetcher

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestIPGuardCheck(t *testing.T) {
	g := newIPGuard(nil, nil)
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"8.8.8.8", false},
		{"2001:4860:4860::8888", false},
		{"127.0.0.1", true},
		{"10.1.2.3", true},
		{"172.20.0.1", true},
		{"192.168.0.1", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true},
		{"0.0.0.0", true},
		{"198.18.0.1", true},
		{"224.0.0.1", true},
		{"255.255.255.255", true},
		{"::", true}, // connect(2) to [::] reaches the local host on Linux
		{"::1", true},
		{"::ffff:127.0.0.1", true},  // IPv4-mapped
		{"::ffff:8.8.8.8", false},   // IPv4-mapped public
		{"fe80::1", true},           // link-local
		{"fe80::1%eth0", true},      // with zone
		{"fc00::1", true},           // ULA outside fd00::/8
		{"fd00:ec2::254", true},     // AWS IMDS over IPv6
		{"ff02::1", true},           // multicast
		{"64:ff9b::a00:1", true},    // NAT64 → 10.0.0.1
		{"64:ff9b::7f00:1", true},   // NAT64 → 127.0.0.1
		{"64:ff9b::808:808", false}, // NAT64 → 8.8.8.8
	}
	for _, tt := range tests {
		err := g.check(netip.MustParseAddr(tt.ip))
		if (err != nil) != tt.blocked {
			t.Errorf("check(%s): err = %v, want blocked=%v", tt.ip, err, tt.blocked)
		}
	}
}

func TestIPGuardAllowedPrivate(t *testing.T) {
	g := newIPGuard([]string{"172.16.0.0/12", "not-a-cidr"}, nil)
	if err := g.check(netip.MustParseAddr("172.18.0.5")); err != nil {
		t.Errorf("allowed private network was blocked: %v", err)
	}
	if err := g.check(netip.MustParseAddr("::ffff:172.18.0.5")); err != nil {
		t.Errorf("IPv4-mapped address in the allowed network was blocked: %v", err)
	}
	if err := g.check(netip.MustParseAddr("10.0.0.1")); err == nil {
		t.Error("10.0.0.1 is outside the allowed network and must stay blocked")
	}
}

// The guard runs in the dialer, so it applies to the address actually being
// connected to, whether the URL holds an IP literal or a name.
func TestFetchBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))

	cfg := Config{Timeout: 5 * time.Second, MaxRedirects: 3, MaxFileSize: 1 << 20}

	for _, u := range []string{
		srv.URL,                             // 127.0.0.1
		"http://localhost:" + port + "/",    // name resolving to loopback
		"http://[::ffff:127.0.0.1]:" + port, // IPv4-mapped literal
	} {
		_, err := New(cfg).Fetch(context.Background(), u)
		if err == nil || !strings.Contains(err.Error(), "ssrf:") {
			t.Errorf("Fetch(%s): err = %v, want an ssrf error", u, err)
		}
	}

	cfg.AllowedPrivateCIDRs = []string{"127.0.0.0/8"}
	res, err := New(cfg).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch with 127.0.0.0/8 allowed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}
