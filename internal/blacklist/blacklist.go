// Package blacklist implements domain and IP blacklisting with optional
// hot-reload from a file via fsnotify.
package blacklist

import (
	"bufio"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
)

// rules holds a parsed snapshot of the blacklist.
type rules struct {
	domains map[string]struct{}
	cidrs   []*net.IPNet
}

// Blacklist checks whether a URL's domain or resolved IP is blocked.
type Blacklist struct {
	current atomic.Pointer[rules]
	file    string
}

// New creates a Blacklist from env-var lists and an optional file path.
// If filePath is non-empty, the file is watched for changes.
func New(domains, ips []string, filePath string) *Blacklist {
	bl := &Blacklist{file: filePath}
	r := parseRules(domains, ips, nil)
	bl.current.Store(r)

	if filePath != "" {
		fileRules := loadFile(filePath)
		merged := mergeRules(r, fileRules)
		bl.current.Store(merged)
		go bl.watch(filePath, domains, ips)
	}
	return bl
}

// IsDenied returns true if the given raw URL's host is blocked.
func (bl *Blacklist) IsDenied(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if bl.DeniesHost(host) {
		return true
	}

	// IP check of the resolved name.
	if net.ParseIP(host) == nil {
		addrs, err := net.LookupHost(host)
		if err != nil || len(addrs) == 0 {
			return false
		}
		if ip, err := netip.ParseAddr(addrs[0]); err == nil {
			return bl.DeniesIP(ip)
		}
	}
	return false
}

// DeniesHost reports whether host (a name or an IP literal, as in
// url.URL.Hostname) is blocked. It never resolves the name: the fetcher
// checks the IPs it actually connects to with DeniesIP.
//
// A blocked domain also blocks its subdomains (blocking example.com blocks
// cdn.example.com, as Misskey's blocked hosts do). Matching ignores case and
// a trailing dot, since "example.com." is the same host as "example.com".
func (bl *Blacklist) DeniesHost(host string) bool {
	r := bl.current.Load()
	if r == nil {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return r.deniesIP(ip)
	}
	name := normalizeDomain(host)
	for name != "" {
		if _, ok := r.domains[name]; ok {
			return true
		}
		i := strings.IndexByte(name, '.')
		if i < 0 {
			break
		}
		name = name[i+1:]
	}
	return false
}

// DeniesIP reports whether ip is in a blocked IP/CIDR.
func (bl *Blacklist) DeniesIP(ip netip.Addr) bool {
	r := bl.current.Load()
	if r == nil {
		return false
	}
	return r.deniesIP(ip)
}

func (r *rules) deniesIP(ip netip.Addr) bool {
	ip = ip.WithZone("").Unmap()
	for _, cidr := range r.cidrs {
		if cidr.Contains(ip.AsSlice()) {
			return true
		}
	}
	return false
}

func normalizeDomain(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}

func (bl *Blacklist) watch(filePath string, envDomains, envIPs []string) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("blacklist: fsnotify init failed", "err", err)
		return
	}
	defer w.Close()

	if err := w.Add(filePath); err != nil {
		slog.Error("blacklist: watch failed", "file", filePath, "err", err)
		return
	}
	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				base := parseRules(envDomains, envIPs, nil)
				file := loadFile(filePath)
				bl.current.Store(mergeRules(base, file))
				slog.Info("blacklist: reloaded", "file", filePath)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			slog.Error("blacklist: watcher error", "err", err)
		}
	}
}

func parseRules(domains, ips []string, extra []string) *rules {
	r := &rules{domains: make(map[string]struct{})}
	for _, d := range domains {
		if d = normalizeDomain(d); d != "" {
			r.domains[d] = struct{}{}
		}
	}
	for _, cidr := range append(ips, extra...) {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Try plain IP.
			if ip := net.ParseIP(cidr); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
			} else {
				continue
			}
		}
		r.cidrs = append(r.cidrs, ipNet)
	}
	return r
}

func loadFile(path string) *rules {
	f, err := os.Open(path)
	if err != nil {
		return &rules{domains: make(map[string]struct{})}
	}
	defer f.Close()

	var domains, cidrs []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "/") || net.ParseIP(line) != nil {
			cidrs = append(cidrs, line)
		} else {
			domains = append(domains, line)
		}
	}
	return parseRules(domains, cidrs, nil)
}

func mergeRules(a, b *rules) *rules {
	merged := &rules{domains: make(map[string]struct{})}
	for d := range a.domains {
		merged.domains[d] = struct{}{}
	}
	for d := range b.domains {
		merged.domains[d] = struct{}{}
	}
	merged.cidrs = append(merged.cidrs, a.cidrs...)
	merged.cidrs = append(merged.cidrs, b.cidrs...)
	return merged
}
