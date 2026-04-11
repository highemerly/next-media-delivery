// Package l1 implements the L1 disk cache backed by a local directory.
// Each cache entry is stored as two files:
//   - {key}.body  — raw response body bytes
//   - {key}.meta  — content-type (single line)
//
// The key is always the hex SHA-256 from the cachekey package.
package l1

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is a successfully retrieved cache item.
type Entry struct {
	Body        []byte
	ContentType string
	// StoredAt is the mtime of the body file, used as Last-Modified for raw responses.
	StoredAt time.Time
}

// Cache is the L1 disk cache.
type Cache struct {
	dir string
}

func New(dir string) *Cache {
	return &Cache{dir: dir}
}

func (c *Cache) bodyPath(key string) string {
	return filepath.Join(c.dir, key+".body")
}

func (c *Cache) metaPath(key string) string {
	return filepath.Join(c.dir, key+".meta")
}

// Get returns the cached entry for the given key, or (nil, nil) on miss.
func (c *Cache) Get(key string) (*Entry, error) {
	bodyPath := c.bodyPath(key)
	metaPath := c.metaPath(key)

	info, err := os.Stat(bodyPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	body, err := os.ReadFile(bodyPath)
	if err != nil {
		return nil, err
	}

	meta, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	return &Entry{
		Body:        body,
		ContentType: strings.TrimSpace(string(meta)),
		StoredAt:    info.ModTime(),
	}, nil
}

// Put atomically writes body and meta files for the given key.
// Writes to a temp file first then renames to avoid partial reads.
func (c *Cache) Put(key string, body []byte, contentType string) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}

	if err := writeAtomic(c.bodyPath(key), body); err != nil {
		return err
	}
	if err := writeAtomic(c.metaPath(key), []byte(contentType)); err != nil {
		return err
	}
	return nil
}

// Delete removes both files for the given key. Missing files are ignored.
func (c *Cache) Delete(key string) error {
	var errs []error
	for _, p := range []string{c.bodyPath(key), c.metaPath(key)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// DirUsage returns the total size in bytes and file count of body files in the cache directory.
func (c *Cache) DirUsage() (totalBytes int64, fileCount int, oldestMtime time.Time, err error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, time.Time{}, nil
		}
		return 0, 0, time.Time{}, err
	}
	oldestMtime = time.Now()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".body") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		totalBytes += info.Size()
		fileCount++
		if info.ModTime().Before(oldestMtime) {
			oldestMtime = info.ModTime()
		}
	}
	return totalBytes, fileCount, oldestMtime, nil
}

// KeyEntry holds metadata for a single cached file.
type KeyEntry struct {
	Mtime time.Time
	Size  int64
}

// KeysWithMtime returns a map of cache key → mtime for all body files.
// Used by the cleanup goroutine and gcOrphans.
func (c *Cache) KeysWithMtime() (map[string]time.Time, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]time.Time{}, nil
		}
		return nil, err
	}
	m := make(map[string]time.Time, len(entries)/2)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".body") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".body")
		m[key] = info.ModTime()
	}
	return m, nil
}

// KeysWithSize returns a map of cache key → KeyEntry (mtime + size) for all body files.
// Used by the cleanup goroutine for accurate eviction accounting.
func (c *Cache) KeysWithSize() (map[string]KeyEntry, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]KeyEntry{}, nil
		}
		return nil, err
	}
	m := make(map[string]KeyEntry, len(entries)/2)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".body") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".body")
		m[key] = KeyEntry{Mtime: info.ModTime(), Size: info.Size()}
	}
	return m, nil
}

func writeAtomic(dst string, data []byte) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	_, writeErr := io.Copy(tmp, strings.NewReader("")) // ensure tmp is writable
	_ = writeErr
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
