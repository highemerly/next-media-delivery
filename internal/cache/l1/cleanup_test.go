package l1

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/highemerly/media-delivery/internal/store"
)

// ---- helpers ----------------------------------------------------------------

// newTestCache creates a Cache backed by a temp directory that is removed when
// the test ends.
func newTestCache(t *testing.T) *Cache {
	t.Helper()
	dir := t.TempDir()
	return New(dir)
}

// putEntry writes a fake cache entry and back-dates its mtime by offset so
// tests can control eviction order without relying on wall-clock timing.
func putEntry(t *testing.T, c *Cache, key string, size int, offset time.Duration) {
	t.Helper()
	body := make([]byte, size)
	if err := c.Put(key, body, "image/jpeg"); err != nil {
		t.Fatalf("putEntry %q: %v", key, err)
	}
	// Back-date the body file so the Cleaner sees a deterministic mtime.
	mtime := time.Now().Add(offset)
	if err := os.Chtimes(c.bodyPath(key), mtime, mtime); err != nil {
		t.Fatalf("putEntry chtimes %q: %v", key, err)
	}
}

// newCleaner builds a Cleaner with the given byte limits.
func newCleaner(c *Cache, tracker store.AccessTracker, maxBytes, tgtBytes int64) *Cleaner {
	return NewCleaner(c, tracker, maxBytes, tgtBytes, time.Hour /* interval unused in tests */)
}

// ---- SumUsage ---------------------------------------------------------------

func TestSumUsage_empty(t *testing.T) {
	total, count, _ := SumUsage(map[string]KeyEntry{})
	if total != 0 || count != 0 {
		t.Errorf("empty map: got total=%d count=%d, want 0 0", total, count)
	}
}

func TestSumUsage(t *testing.T) {
	now := time.Now()
	older := now.Add(-10 * time.Minute)
	keys := map[string]KeyEntry{
		"a": {Mtime: now, Size: 100},
		"b": {Mtime: older, Size: 200},
	}
	total, count, oldest := SumUsage(keys)
	if total != 300 {
		t.Errorf("total: got %d, want 300", total)
	}
	if count != 2 {
		t.Errorf("count: got %d, want 2", count)
	}
	if !oldest.Equal(older) {
		t.Errorf("oldest: got %v, want %v", oldest, older)
	}
}

// ---- KeysWithSize -----------------------------------------------------------

func TestKeysWithSize_empty(t *testing.T) {
	c := newTestCache(t)
	keys, err := c.KeysWithSize()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty map, got %v", keys)
	}
}

func TestKeysWithSize(t *testing.T) {
	c := newTestCache(t)
	putEntry(t, c, "key1", 100, 0)
	putEntry(t, c, "key2", 200, 0)

	keys, err := c.KeysWithSize()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys["key1"].Size != 100 {
		t.Errorf("key1 size: got %d, want 100", keys["key1"].Size)
	}
	if keys["key2"].Size != 200 {
		t.Errorf("key2 size: got %d, want 200", keys["key2"].Size)
	}
}

// ---- Cleaner.runOnce: under threshold ---------------------------------------

// Under threshold: no files should be deleted, orphan GC should still run.
func TestRunOnce_underThreshold(t *testing.T) {
	c := newTestCache(t)
	tracker := store.NewMemoryTracker()
	putEntry(t, c, "key1", 100, 0)
	putEntry(t, c, "key2", 100, 0)
	// total = 200 bytes, maxBytes = 1000 → no eviction expected
	cl := newCleaner(c, tracker, 1000, 500)
	cl.runOnce(context.Background())

	keys, _ := c.KeysWithSize()
	if len(keys) != 2 {
		t.Errorf("expected 2 files after runOnce, got %d", len(keys))
	}
}

// ---- Cleaner.runOnce: eviction ----------------------------------------------

// Over threshold: oldest files should be evicted until usage drops to tgtBytes.
func TestRunOnce_evictsOldestFirst(t *testing.T) {
	c := newTestCache(t)
	tracker := store.NewMemoryTracker()

	// Three entries of 100 bytes each = 300 bytes total.
	// oldest → newest: key_old, key_mid, key_new
	putEntry(t, c, "key_old", 100, -2*time.Hour)
	putEntry(t, c, "key_mid", 100, -1*time.Hour)
	putEntry(t, c, "key_new", 100, 0)

	// maxBytes=250, tgtBytes=150 → need to free ≥150 bytes.
	// Eviction order: key_old (100) → remaining 200; key_mid (100) → remaining 100 ≤ 150. Stop.
	cl := newCleaner(c, tracker, 250, 150)
	cl.runOnce(context.Background())

	keys, _ := c.KeysWithSize()
	if _, ok := keys["key_old"]; ok {
		t.Error("key_old should have been evicted")
	}
	if _, ok := keys["key_mid"]; ok {
		t.Error("key_mid should have been evicted")
	}
	if _, ok := keys["key_new"]; !ok {
		t.Error("key_new should have been kept")
	}
}

// Evicted keys must also be removed from the tracker.
func TestRunOnce_evictionRemovesTrackerEntries(t *testing.T) {
	c := newTestCache(t)
	tracker := store.NewMemoryTracker()
	ctx := context.Background()

	putEntry(t, c, "key_old", 100, -1*time.Hour)
	putEntry(t, c, "key_new", 100, 0)
	tracker.Set(ctx, "key_old", time.Now()) //nolint:errcheck
	tracker.Set(ctx, "key_new", time.Now()) //nolint:errcheck

	// maxBytes=150, tgtBytes=110 → evict key_old (100 bytes); remaining=100 ≤ 110, stop.
	// key_new must survive and stay in tracker.
	cl := newCleaner(c, tracker, 150, 110)
	cl.runOnce(ctx)

	if tracker.Len() != 1 {
		t.Errorf("tracker should have 1 entry after eviction, got %d", tracker.Len())
	}
}

// ---- Cleaner.runOnce: orphan GC ---------------------------------------------

// Tracker entries with no corresponding disk file should be removed.
func TestRunOnce_gcOrphans(t *testing.T) {
	c := newTestCache(t)
	tracker := store.NewMemoryTracker()
	ctx := context.Background()

	putEntry(t, c, "key_exists", 100, 0)
	// "key_orphan" is in the tracker but has no file on disk.
	tracker.Set(ctx, "key_exists", time.Now()) //nolint:errcheck
	tracker.Set(ctx, "key_orphan", time.Now()) //nolint:errcheck

	// maxBytes is large enough that no eviction happens; only orphan GC runs.
	cl := newCleaner(c, tracker, 10000, 5000)
	cl.runOnce(ctx)

	if tracker.Len() != 1 {
		t.Errorf("tracker should have 1 entry after orphan GC, got %d", tracker.Len())
	}
	// key_exists must still be present
	_, ok, _ := tracker.Get(ctx, "key_exists")
	if !ok {
		t.Error("key_exists should still be in tracker")
	}
}

// Orphan GC must also run after eviction (evicted keys must not be re-added as orphans).
func TestRunOnce_gcOrphansAfterEviction(t *testing.T) {
	c := newTestCache(t)
	tracker := store.NewMemoryTracker()
	ctx := context.Background()

	putEntry(t, c, "key_old", 100, -1*time.Hour)
	putEntry(t, c, "key_new", 100, 0)
	tracker.Set(ctx, "key_old", time.Now())    //nolint:errcheck
	tracker.Set(ctx, "key_new", time.Now())    //nolint:errcheck
	tracker.Set(ctx, "key_orphan", time.Now()) //nolint:errcheck

	// maxBytes=150, tgtBytes=110 → key_old evicted; key_orphan should also be cleaned up
	cl := newCleaner(c, tracker, 150, 110)
	cl.runOnce(ctx)

	// Only key_new should remain
	if tracker.Len() != 1 {
		t.Errorf("tracker should have 1 entry, got %d", tracker.Len())
	}
	_, ok, _ := tracker.Get(ctx, "key_new")
	if !ok {
		t.Error("key_new should still be in tracker")
	}
}
