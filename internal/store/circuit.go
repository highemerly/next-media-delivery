package store

import (
	"context"
	"sync"
	"time"
)

type CircuitState int

const (
	StateClosed   CircuitState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF-OPEN"
	default:
		return "CLOSED"
	}
}

// CircuitEntry holds per-domain circuit breaker state.
type CircuitEntry struct {
	State      CircuitState
	Failures   int
	LastFailed time.Time
	OpenUntil  time.Time
}

// CircuitBreaker guards outbound fetches per domain.
type CircuitBreaker interface {
	// Allow returns true if the domain may be fetched right now.
	Allow(ctx context.Context, domain string) (bool, error)
	RecordSuccess(ctx context.Context, domain string) error
	RecordFailure(ctx context.Context, domain string) error
	// Snapshot returns all current entries (used by stats).
	Snapshot(ctx context.Context) (map[string]CircuitEntry, error)
	// GC removes stale CLOSED entries. No-op for Redis implementation.
	GC(ctx context.Context) (removed int, err error)
}

// NoopCircuitBreaker always allows requests. Used in Phase 1.
type NoopCircuitBreaker struct{}

func NewNoopCircuitBreaker() *NoopCircuitBreaker { return &NoopCircuitBreaker{} }

func (n *NoopCircuitBreaker) Allow(_ context.Context, _ string) (bool, error)        { return true, nil }
func (n *NoopCircuitBreaker) RecordSuccess(_ context.Context, _ string) error         { return nil }
func (n *NoopCircuitBreaker) RecordFailure(_ context.Context, _ string) error         { return nil }
func (n *NoopCircuitBreaker) Snapshot(_ context.Context) (map[string]CircuitEntry, error) {
	return map[string]CircuitEntry{}, nil
}
func (n *NoopCircuitBreaker) GC(_ context.Context) (int, error) { return 0, nil }

// MemoryCircuitBreaker is a full in-process CircuitBreaker implementation.
type MemoryCircuitBreaker struct {
	mu        sync.Mutex
	data      map[string]*CircuitEntry
	threshold int
	timeout   time.Duration
}

func NewMemoryCircuitBreaker(threshold int, timeout time.Duration) *MemoryCircuitBreaker {
	return &MemoryCircuitBreaker{
		data:      make(map[string]*CircuitEntry),
		threshold: threshold,
		timeout:   timeout,
	}
}

func (m *MemoryCircuitBreaker) Allow(_ context.Context, domain string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.data[domain]
	if !ok {
		return true, nil
	}
	switch e.State {
	case StateOpen:
		if time.Now().After(e.OpenUntil) {
			e.State = StateHalfOpen
			return true, nil
		}
		return false, nil
	case StateHalfOpen:
		// Allow one probe through (not strictly exclusive; see design note).
		return true, nil
	default:
		return true, nil
	}
}

func (m *MemoryCircuitBreaker) RecordSuccess(_ context.Context, domain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, domain)
	return nil
}

func (m *MemoryCircuitBreaker) RecordFailure(_ context.Context, domain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.data[domain]
	if !ok {
		e = &CircuitEntry{State: StateClosed}
		m.data[domain] = e
	}
	e.Failures++
	e.LastFailed = time.Now()
	if e.Failures >= m.threshold {
		e.State = StateOpen
		e.OpenUntil = time.Now().Add(m.timeout)
	}
	return nil
}

func (m *MemoryCircuitBreaker) Snapshot(_ context.Context) (map[string]CircuitEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]CircuitEntry, len(m.data))
	for k, v := range m.data {
		out[k] = *v
	}
	return out, nil
}

func (m *MemoryCircuitBreaker) GC(_ context.Context) (int, error) {
	cutoff := time.Now().Add(-24 * time.Hour)
	m.mu.Lock()
	removed := 0
	for k, e := range m.data {
		if e.State == StateClosed && e.LastFailed.Before(cutoff) {
			delete(m.data, k)
			removed++
		}
	}
	m.mu.Unlock()
	return removed, nil
}
