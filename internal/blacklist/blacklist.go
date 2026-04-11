// Package blacklist implements domain and IP blacklisting with optional
// hot-reload from a file via fsnotify.
package blacklist

import (
	"bufio"
	"log/slog"
	"net"
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
	r := bl.current.Load()
	if r == nil {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()

	// Domain check.
	if _, ok := r.domains[strings.ToLower(host)]; ok {
		return true
	}

	// IP check (direct IP in URL or resolved).
	ip := net.ParseIP(host)
	if ip == nil {
		addrs, err := net.LookupHost(host)
		if err != nil || len(addrs) == 0 {
			return false
		}
		ip = net.ParseIP(addrs[0])
	}
	if ip != nil {
		for _, cidr := range r.cidrs {
			if cidr.Contains(ip) {
				return true
			}
		}
	}
	return false
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
		r.domains[strings.ToLower(strings.TrimSpace(d))] = struct{}{}
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
