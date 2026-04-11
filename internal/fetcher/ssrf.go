package fetcher

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// blockedCIDRs are always denied unless overridden by AllowedPrivateCIDRs.
var blockedCIDRs = mustParseCIDRs([]string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.0/8",
	"::1/128",
	"169.254.0.0/16",
	"fd00::/8",
	"100.64.0.0/10", // Carrier-grade NAT
	"0.0.0.0/8",
	"240.0.0.0/4",
})

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, ipNet, err := net.ParseCIDR(c)
		if err != nil {
			panic("invalid built-in CIDR: " + c)
		}
		nets = append(nets, ipNet)
	}
	return nets
}

// ssrfTransport wraps http.Transport with a custom DialContext that enforces SSRF rules.
type ssrfTransport struct {
	base    *http.Transport
	allowed []*net.IPNet // CIDRs that are exempted from the block list
}

func newSSRFTransport(allowedPrivateCIDRs []string) *ssrfTransport {
	allowed := make([]*net.IPNet, 0, len(allowedPrivateCIDRs))
	for _, cidr := range allowedPrivateCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		allowed = append(allowed, ipNet)
	}

	base := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
	}

	t := &ssrfTransport{base: base, allowed: allowed}
	base.DialContext = t.dialContext
	return t
}

func (t *ssrfTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req)
}

// dialContext resolves the hostname and validates the IP before dialing.
// This prevents DNS rebinding by using the resolved IP directly.
func (t *ssrfTransport) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("ssrf: invalid addr %q: %w", addr, err)
	}

	// Reject direct IP addresses (no hostname to resolve).
	if ip := net.ParseIP(host); ip != nil {
		if err := t.checkIP(ip); err != nil {
			return nil, err
		}
		d := &net.Dialer{Timeout: 10 * time.Second}
		return d.DialContext(ctx, network, addr)
	}

	// Resolve hostname.
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("ssrf: dns lookup failed for %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("ssrf: no addresses resolved for %q", host)
	}

	// Validate all resolved IPs.
	for _, a := range addrs {
		if err := t.checkIP(a.IP); err != nil {
			return nil, err
		}
	}

	// Dial the first resolved IP directly (prevents DNS rebinding).
	resolvedAddr := net.JoinHostPort(addrs[0].IP.String(), port)
	d := &net.Dialer{Timeout: 10 * time.Second}
	return d.DialContext(ctx, network, resolvedAddr)
}

func (t *ssrfTransport) checkIP(ip net.IP) error {
	// Check allow list first.
	for _, allowed := range t.allowed {
		if allowed.Contains(ip) {
			return nil
		}
	}
	// Check block list.
	for _, blocked := range blockedCIDRs {
		if blocked.Contains(ip) {
			return fmt.Errorf("ssrf: IP %s is in blocked range %s", ip, blocked)
		}
	}
	return nil
}
