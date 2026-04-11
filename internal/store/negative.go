package store

import (
	"context"
	"sync"
	"time"
)

// NegativeEntry records an error response to avoid re-fetching dead URLs.
type NegativeEntry struct {
	Status   int       // 4xx or 5xx
	ExpireAt time.Time
}

// NegativeCache stores negative cache entries keyed by cache key.
type NegativeCache interface {
	Set(ctx context.Context, key string, entry NegativeEntry) error
	Get(ctx context.Context, key string) (NegativeEntry, bool, error)
	// GC removes expired entries. No-op for Redis implementation.
	GC(ctx context.Context) (removed int, err error)
	// Len returns the number of stored entries.
	Len() int
}

// MemoryNegativeCache is an in-process NegativeCache backed by a plain map.
type MemoryNegativeCache struct {
	mu   sync.RWMutex
	data map[string]NegativeEntry
}

func NewMemoryNegativeCache() *MemoryNegativeCache {
	return &MemoryNegativeCache{data: make(map[string]NegativeEntry)}
}

func (m *MemoryNegativeCache) Set(_ context.Context, key string, entry NegativeEntry) error {
	m.mu.Lock()
	m.data[key] = entry
	m.mu.Unlock()
	return nil
}

func (m *MemoryNegativeCache) Get(_ context.Context, key string) (NegativeEntry, bool, error) {
	m.mu.RLock()
	entry, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return NegativeEntry{}, false, nil
	}
	if time.Now().After(entry.ExpireAt) {
		return NegativeEntry{}, false, nil
	}
	return entry, true, nil
}

func (m *MemoryNegativeCache) GC(_ context.Context) (int, error) {
	now := time.Now()
	m.mu.Lock()
	removed := 0
	for k, e := range m.data {
		if now.After(e.ExpireAt) {
			delete(m.data, k)
			removed++
		}
	}
	m.mu.Unlock()
	return removed, nil
}

func (m *MemoryNegativeCache) Len() int {
	m.mu.RLock()
	n := len(m.data)
	m.mu.RUnlock()
	return n
}

// Snapshot returns a copy of all entries (including expired ones).
// Used by the admin stats endpoint.
func (m *MemoryNegativeCache) Snapshot() map[string]NegativeEntry {
	m.mu.RLock()
	out := make(map[string]NegativeEntry, len(m.data))
	for k, v := range m.data {
		out[k] = v
	}
	m.mu.RUnlock()
	return out
}
