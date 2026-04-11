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
	totalBytes, _, _, err := cl.cache.DirUsage()
	if err != nil {
		slog.Error("l1 cleanup: failed to read dir usage", "err", err)
		return
	}

	if totalBytes <= cl.maxBytes {
		// Under threshold — only GC orphan tracker entries.
		cl.gcOrphans(ctx)
		return
	}

	slog.Info("l1 cleanup: disk over threshold, evicting",
		"used_bytes", totalBytes,
		"max_bytes", cl.maxBytes,
		"target_bytes", cl.tgtBytes,
	)

	keysWithSize, err := cl.cache.KeysWithSize()
	if err != nil {
		slog.Error("l1 cleanup: failed to list keys", "err", err)
		return
	}

	// Sort keys oldest-mtime first.
	type kv struct {
		key   string
		mtime time.Time
		size  int64
	}
	sorted := make([]kv, 0, len(keysWithSize))
	for k, e := range keysWithSize {
		sorted = append(sorted, kv{k, e.Mtime, e.Size})
	}
	sort.Slice(sorted, func(i, j int) bool {
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
	}

	cl.gcOrphans(ctx)
}

// gcOrphans removes tracker entries for keys that no longer exist on disk.
func (cl *Cleaner) gcOrphans(ctx context.Context) {
	keysWithSize, err := cl.cache.KeysWithSize()
	if err != nil {
		return
	}
	existing := make(map[string]struct{}, len(keysWithSize))
	for k := range keysWithSize {
		existing[k] = struct{}{}
	}
	if mt, ok := cl.tracker.(*store.MemoryTracker); ok {
		removed := mt.GCOrphans(ctx, existing)
		if removed > 0 {
			slog.Info("l1 cleanup: gc orphans", "removed", removed)
		}
	}
}

