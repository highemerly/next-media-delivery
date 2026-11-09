package fetcher

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// blockedPrefixes are always denied unless overridden by AllowedPrivateCIDRs.
var blockedPrefixes = mustParsePrefixes([]string{
	// IPv4
	"0.0.0.0/8",      // "this network"; 0.0.0.0 reaches the local host
	"10.0.0.0/8",     // private
	"100.64.0.0/10",  // carrier-grade NAT
	"127.0.0.0/8",    // loopback
	"169.254.0.0/16", // link-local, cloud metadata
	"172.16.0.0/12",  // private
	"192.0.0.0/24",   // IETF protocol assignments
	"192.168.0.0/16", // private
	"198.18.0.0/15",  // benchmarking
	"224.0.0.0/4",    // multicast
	"240.0.0.0/4",    // reserved, broadcast
	// IPv6 (IPv4-mapped addresses are unmapped and checked as IPv4)
	"::/128",    // unspecified; connecting to [::] reaches the local host
	"::1/128",   // loopback
	"fc00::/7",  // unique local (includes fd00::/8, e.g. AWS fd00:ec2::254)
	"fe80::/10", // link-local
	"ff00::/8",  // multicast
})

// nat64Prefix is the well-known NAT64 prefix (RFC 6052). An address in it
// carries an IPv4 address in its last 32 bits, which is what the NAT64
// gateway connects to, so that IPv4 address is checked instead. Blocking the
// whole prefix would break every IPv4-only origin on an IPv6-only network
// with DNS64.
var nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")

func mustParsePrefixes(cidrs []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// ipGuard decides whether an origin IP may be connected to.
type ipGuard struct {
	allowed []netip.Prefix // exempted from blockedPrefixes
	deny    Denylist       // optional origin blacklist (IP/CIDR rules)
}

func newIPGuard(allowedPrivateCIDRs []string, deny Denylist) *ipGuard {
	g := &ipGuard{deny: deny}
	for _, cidr := range allowedPrivateCIDRs {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			slog.Warn("ssrf: ignoring invalid ORIGIN_ALLOWED_PRIVATE_NETWORKS entry", "cidr", cidr, "err", err)
			continue
		}
		g.allowed = append(g.allowed, p.Masked())
	}
	return g
}

func (g *ipGuard) check(ip netip.Addr) error {
	ip = ip.WithZone("").Unmap()
	for _, p := range g.allowed {
		if p.Contains(ip) {
			return nil
		}
	}
	if nat64Prefix.Contains(ip) {
		b := ip.As16()
		return g.check(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return fmt.Errorf("ssrf: IP %s is in blocked range %s", ip, p)
		}
	}
	return nil
}

// control runs after DNS resolution and immediately before each connect(2),
// with the exact IP being connected to. Checking here (rather than resolving
// the name ourselves first) leaves no window for DNS rebinding, covers every
// address the dialer tries (Happy Eyeballs, fallbacks, redirects), and does
// not apply to the resolver's own connections to the DNS server.
func (g *ipGuard) control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("ssrf: invalid address %q: %w", address, err)
	}
	if g.deny != nil && g.deny.DeniesIP(ap.Addr()) {
		return fmt.Errorf("%w: IP %s", ErrDenied, ap.Addr())
	}
	return g.check(ap.Addr())
}

// ssrfTransport wraps http.Transport with a dialer that enforces SSRF rules.
type ssrfTransport struct {
	base  *http.Transport
	guard *ipGuard
}

func newSSRFTransport(allowedPrivateCIDRs []string, deny Denylist) *ssrfTransport {
	guard := newIPGuard(allowedPrivateCIDRs, deny)

	base := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   guard.control,
	}
	base.DialContext = dialer.DialContext
	return &ssrfTransport{base: base, guard: guard}
}

func (t *ssrfTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req)
}
