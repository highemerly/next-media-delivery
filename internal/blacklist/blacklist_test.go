package blacklist

import (
	"net/netip"
	"testing"
)

func TestDeniesHost(t *testing.T) {
	bl := New([]string{"Blocked.Example", "spam.example.org."}, []string{"203.0.113.1", "198.51.100.0/24", "2001:db8::/32"}, "")
	tests := []struct {
		host string
		want bool
	}{
		{"blocked.example", true},
		{"BLOCKED.example", true},
		{"blocked.example.", true},     // trailing dot is the same host
		{"cdn.blocked.example", true},  // subdomain
		{"a.b.blocked.example.", true}, // nested subdomain + trailing dot
		{"notblocked.example", false},  // suffix match is per label
		{"blocked.example.net", false}, // different domain
		{"spam.example.org", true},     // trailing dot in the rule itself
		{"example.org", false},         // parent of a blocked domain
		{"203.0.113.1", true},          // IP literal
		{"198.51.100.200", true},       // IP literal inside a CIDR
		{"203.0.113.2", false},
		{"::ffff:203.0.113.1", true}, // IPv4-mapped literal
		{"2001:db8::1", true},
		{"", false},
	}
	for _, tt := range tests {
		if got := bl.DeniesHost(tt.host); got != tt.want {
			t.Errorf("DeniesHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestDeniesIP(t *testing.T) {
	bl := New(nil, []string{"198.51.100.0/24"}, "")
	if !bl.DeniesIP(netip.MustParseAddr("198.51.100.7")) {
		t.Error("198.51.100.7 should be denied")
	}
	if !bl.DeniesIP(netip.MustParseAddr("::ffff:198.51.100.7")) {
		t.Error("IPv4-mapped 198.51.100.7 should be denied")
	}
	if bl.DeniesIP(netip.MustParseAddr("192.0.2.1")) {
		t.Error("192.0.2.1 should be allowed")
	}
}

func TestIsDeniedTrailingDot(t *testing.T) {
	bl := New([]string{"blocked.example"}, nil, "")
	if !bl.IsDenied("http://blocked.example./a.png") {
		t.Error("a trailing dot must not bypass the domain blacklist")
	}
}

// IsDenied must not resolve names (it runs on every request, cache hits
// included). A name that resolves to a blocked IP is refused by the fetcher
// when it connects, not here.
func TestIsDeniedDoesNotResolve(t *testing.T) {
	bl := New(nil, []string{"127.0.0.0/8", "::1/128"}, "")
	if bl.IsDenied("http://localhost/a.png") {
		t.Error("IsDenied resolved localhost; IP rules for names belong to the fetcher's dialer")
	}
	if !bl.IsDenied("http://127.0.0.1/a.png") {
		t.Error("an IP literal in a blocked range must still be denied up front")
	}
}
