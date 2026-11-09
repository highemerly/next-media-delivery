package l1

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/highemerly/media-delivery/internal/store"
)

// Cleaner runs periodic L1 disk eviction and AccessTracker GC.
type Cleaner struct {
	cache    *Cache
	tracker  store.AccessTracker
	maxBytes int64
	tgtBytes int64
	interval time.Duration
}

func NewCleaner(cache *Cache, tracker store.AccessTracker, maxBytes, tgtBytes int64, interval time.Duration) *Cleaner {
	return &Cleaner{
		cache:    cache,
		tracker:  tracker,
		maxBytes: maxBytes,
		tgtBytes: tgtBytes,
		interval: interval,
	}
}

// Run starts the cleanup loop. It exits when ctx is cancelled.
func (cl *Cleaner) Run(ctx context.Context) {
	ticker := time.NewTicker(cl.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cl.runOnce(ctx)
		}
	}
}

func (cl *Cleaner) runOnce(ctx context.Context) {
	// Single disk scan: both the threshold check and eviction use this result.
	keysWithSize, err := cl.cache.KeysWithSize()
	if err != nil {
		slog.Error("l1 cleanup: failed to list keys", "err", err)
		return
	}
	totalBytes, _, _ := SumUsage(keysWithSize)

	if totalBytes <= cl.maxBytes {
		// Under threshold — only GC orphan tracker entries.
		cl.gcOrphans(ctx, keysWithSize)
		return
	}

	slog.Info("l1 cleanup: disk over threshold, evicting",
		"used_bytes", totalBytes,
		"max_bytes", cl.maxBytes,
		"target_bytes", cl.tgtBytes,
	)

	// Sort keys least-recently-used first. The last access comes from the
	// AccessTracker (updated on every L1/L2 hit and origin fetch); the file
	// mtime is only the write time, so sorting by it alone evicted popular
	// entries as soon as they got old. Keys the tracker does not know (e.g.
	// after a restart) fall back to their mtime.
	type kv struct {
		key        string
		lastAccess time.Time
		mtime      time.Time
		size       int64
	}
	sorted := make([]kv, 0, len(keysWithSize))
	for k, e := range keysWithSize {
		sorted = append(sorted, kv{k, cl.lastAccess(ctx, k, e.Mtime), e.Mtime, e.Size})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if !sorted[i].lastAccess.Equal(sorted[j].lastAccess) {
			return sorted[i].lastAccess.Before(sorted[j].lastAccess)
		}
		return sorted[i].mtime.Before(sorted[j].mtime)
	})

	var deleted []string
	remaining := totalBytes
	for _, entry := range sorted {
		if remaining <= cl.tgtBytes {
			break
		}
		if err := cl.cache.Delete(entry.key); err != nil {
			slog.Error("l1 cleanup: failed to delete file", "key", entry.key, "err", err)
			continue
		}
		deleted = append(deleted, entry.key)
		remaining -= entry.size
	}

	if len(deleted) > 0 {
		if err := cl.tracker.DeleteKeys(ctx, deleted); err != nil {
			slog.Error("l1 cleanup: failed to delete tracker entries", "err", err)
		}
		slog.Info("l1 cleanup: eviction complete", "deleted", len(deleted))
		// Remove evicted keys from the map so gcOrphans works on current state.
		for _, k := range deleted {
			delete(keysWithSize, k)
		}
	}

	cl.gcOrphans(ctx, keysWithSize)
}

// lastAccess returns the later of the tracker's last access time for key and
// the file's mtime (the tracker update and the file write race on a fresh
// entry, so either can be slightly ahead).
func (cl *Cleaner) lastAccess(ctx context.Context, key string, mtime time.Time) time.Time {
	t, ok, err := cl.tracker.Get(ctx, key)
	if err != nil || !ok || t.Before(mtime) {
		return mtime
	}
	return t
}

// gcOrphans removes tracker entries for keys that no longer exist on disk.
// keys is the already-scanned KeysWithSize result from the caller.
func (cl *Cleaner) gcOrphans(ctx context.Context, keys map[string]KeyEntry) {
	existing := make(map[string]struct{}, len(keys))
	for k := range keys {
		existing[k] = struct{}{}
	}
	if mt, ok := cl.tracker.(*store.MemoryTracker); ok {
		removed := mt.GCOrphans(ctx, existing)
		if removed > 0 {
			slog.Info("l1 cleanup: gc orphans", "removed", removed)
		}
	}
}

