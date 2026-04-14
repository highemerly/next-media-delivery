package main

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/highemerly/media-delivery/internal/cache/l1"
	"github.com/highemerly/media-delivery/internal/cache/l2"
	"github.com/highemerly/media-delivery/internal/config"
	"github.com/highemerly/media-delivery/internal/store"
)

// --- /stats ---

type statsResponse struct {
	L1             l1Stats             `json:"l1"`
	L2             l2Stats             `json:"l2"`
	AccessTracker  trackerStats        `json:"access_tracker"`
	NegativeCache  negativeCacheStats  `json:"negative_cache"`
	CircuitBreaker circuitBreakerStats `json:"circuit_breaker"`
}

type l1Stats struct {
	UsedBytes  int64     `json:"used_bytes"`
	FileCount  int       `json:"file_count"`
	MaxBytes   int64     `json:"max_bytes"`
	OldestFile time.Time `json:"oldest_file,omitempty"`
}

type l2Stats struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint,omitempty"`
}

type trackerStats struct {
	Entries int    `json:"entries"`
	Backend string `json:"backend"`
}

type negativeCacheStats struct {
	Entries int    `json:"entries"`
	Backend string `json:"backend"`
}

type circuitBreakerStats struct {
	Closed   int    `json:"closed"`
	Open     int    `json:"open"`
	HalfOpen int    `json:"half_open"`
	Backend  string `json:"backend"`
}

func makeStatsHandler(
	l1Cache *l1.Cache,
	tracker store.AccessTracker,
	negCache store.NegativeCache,
	circuit store.CircuitBreaker,
	cfg *config.Config,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keys, _ := l1Cache.KeysWithSize()
		usedBytes, fileCount, oldestMtime := l1.SumUsage(keys)

		snapshot, _ := circuit.Snapshot(r.Context())
		closed, open, halfOpen := 0, 0, 0
		for _, e := range snapshot {
			switch e.State {
			case store.StateOpen:
				open++
			case store.StateHalfOpen:
				halfOpen++
			default:
				closed++
			}
		}

		resp := statsResponse{
			L1: l1Stats{
				UsedBytes:  usedBytes,
				FileCount:  fileCount,
				MaxBytes:   cfg.Cache.MaxBytes,
				OldestFile: oldestMtime,
			},
			L2: l2Stats{
				Enabled:  cfg.S3.Enabled,
				Endpoint: cfg.S3.Endpoint,
			},
			AccessTracker: trackerStats{
				Entries: tracker.Len(),
				Backend: cfg.Store.Backend,
			},
			NegativeCache: negativeCacheStats{
				Entries: negCache.Len(),
				Backend: cfg.Store.Backend,
			},
			CircuitBreaker: circuitBreakerStats{
				Closed:   closed,
				Open:     open,
				HalfOpen: halfOpen,
				Backend:  cfg.Store.Backend,
			},
		}

		writeJSON(w, resp)
	}
}

// --- /stats/circuit-breaker ---

type circuitBreakerEntry struct {
	Domain    string    `json:"domain"`
	State     string    `json:"state"`
	Failures  int       `json:"failures"`
	OpenUntil time.Time `json:"open_until,omitempty"`
}

func makeCircuitBreakerStatsHandler(circuit store.CircuitBreaker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := circuit.Snapshot(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entries := make([]circuitBreakerEntry, 0, len(snapshot))
		for domain, e := range snapshot {
			entries = append(entries, circuitBreakerEntry{
				Domain:    domain,
				State:     e.State.String(),
				Failures:  e.Failures,
				OpenUntil: e.OpenUntil,
			})
		}
		writeJSON(w, entries)
	}
}

// --- /stats/negative-cache ---

type negativeCacheEntry struct {
	Key      string    `json:"key"`
	Status   int       `json:"status"`
	ExpireAt time.Time `json:"expire_at"`
}

func makeNegativeCacheStatsHandler(negCache store.NegativeCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// MemoryNegativeCache exposes a snapshot via type assertion.
		type snapshotter interface {
			Snapshot() map[string]store.NegativeEntry
		}
		snap, ok := negCache.(snapshotter)
		if !ok {
			writeJSON(w, []negativeCacheEntry{})
			return
		}
		raw := snap.Snapshot()
		entries := make([]negativeCacheEntry, 0, len(raw))
		for key, e := range raw {
			entries = append(entries, negativeCacheEntry{
				Key:      key,
				Status:   e.Status,
				ExpireAt: e.ExpireAt,
			})
		}
		writeJSON(w, entries)
	}
}

// --- DELETE /cache/{key} ---

func makePurgeHandler(l1Cache *l1.Cache, l2Store l2.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		key := strings.TrimPrefix(path.Base(r.URL.Path), "/")
		if key == "" || key == "cache" {
			http.Error(w, "missing cache key", http.StatusBadRequest)
			return
		}
		if err := l1Cache.Delete(key); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if l2Store.Enabled() {
			if err := l2Store.Delete(r.Context(), key); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- DELETE /cache (purge all) ---

func makePurgeAllHandler(l1Cache *l1.Cache, l2Store l2.Store, tracker store.AccessTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		keys, err := l1Cache.KeysWithSize()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		keyList := make([]string, 0, len(keys))
		for k := range keys {
			l1Cache.Delete(k) //nolint:errcheck
			if l2Store.Enabled() {
				l2Store.Delete(r.Context(), k) //nolint:errcheck
			}
			keyList = append(keyList, k)
		}
		tracker.DeleteKeys(r.Context(), keyList) //nolint:errcheck

		writeJSON(w, map[string]int{"deleted": len(keyList)})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
