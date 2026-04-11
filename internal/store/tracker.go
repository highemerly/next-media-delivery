package store

import (
	"context"
	"sort"
	"sync"
	"time"
)

// AccessTracker tracks the last access time per cache key.
// Used by L1 cleanup to find eviction candidates and by L2 lifecycle renewal.
type AccessTracker interface {
	Set(ctx context.Context, key string, t time.Time) error
	Get(ctx context.Context, key string) (time.Time, bool, error)
	// OldestFirst returns up to limit keys sorted by access time ascending.
	OldestFirst(ctx context.Context, limit int) ([]string, error)
	// DeleteKeys removes entries for the given keys (called when L1 evicts files).
	DeleteKeys(ctx context.Context, keys []string) error
	// Len returns the number of tracked entries.
	Len() int
}

// MemoryTracker is an in-process AccessTracker backed by a plain map.
type MemoryTracker struct {
	mu   sync.RWMutex
	data map[string]time.Time
}

func NewMemoryTracker() *MemoryTracker {
	return &MemoryTracker{data: make(map[string]time.Time)}
}

func (m *MemoryTracker) Set(_ context.Context, key string, t time.Time) error {
	m.mu.Lock()
	m.data[key] = t
	m.mu.Unlock()
	return nil
}

func (m *MemoryTracker) Get(_ context.Context, key string) (time.Time, bool, error) {
	m.mu.RLock()
	t, ok := m.data[key]
	m.mu.RUnlock()
	return t, ok, nil
}

func (m *MemoryTracker) OldestFirst(_ context.Context, limit int) ([]string, error) {
	m.mu.RLock()
	type entry struct {
		key string
		t   time.Time
	}
	entries := make([]entry, 0, len(m.data))
	for k, t := range m.data {
		entries = append(entries, entry{k, t})
	}
	m.mu.RUnlock()

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].t.Before(entries[j].t)
	})

	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = e.key
	}
	return keys, nil
}

func (m *MemoryTracker) DeleteKeys(_ context.Context, keys []string) error {
	m.mu.Lock()
	for _, k := range keys {
		delete(m.data, k)
	}
	m.mu.Unlock()
	return nil
}

func (m *MemoryTracker) Len() int {
	m.mu.RLock()
	n := len(m.data)
	m.mu.RUnlock()
	return n
}

// GCOrphans removes entries whose keys are not present in the given set.
// Called by L1 cleanup to remove stale tracker entries for deleted files.
func (m *MemoryTracker) GCOrphans(_ context.Context, existingKeys map[string]struct{}) int {
	m.mu.Lock()
	removed := 0
	for k := range m.data {
		if _, ok := existingKeys[k]; !ok {
			delete(m.data, k)
			removed++
		}
	}
	m.mu.Unlock()
	return removed
}
