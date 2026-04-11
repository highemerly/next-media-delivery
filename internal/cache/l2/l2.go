// Package l2 defines the L2 object storage interface and a no-op implementation.
// The S3 implementation lives in s3.go and is wired in when S3_ENABLED=true.
package l2

import (
	"context"
	"io"
	"time"
)

// Object is a retrieved L2 cache item.
type Object struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
	StoredAt    time.Time
}

// Store is the L2 cache abstraction.
type Store interface {
	// Get returns the object for the given key, or (nil, nil) on miss.
	Get(ctx context.Context, key string) (*Object, error)
	Put(ctx context.Context, key string, r io.Reader, contentType string, size int64) error
	Delete(ctx context.Context, key string) error
	// Enabled returns false for NoopStore (S3_ENABLED=false).
	Enabled() bool
}

// NoopStore is used when S3_ENABLED=false. All operations are no-ops.
type NoopStore struct{}

func NewNoopStore() *NoopStore { return &NoopStore{} }

func (n *NoopStore) Get(_ context.Context, _ string) (*Object, error)                          { return nil, nil }
func (n *NoopStore) Put(_ context.Context, _ string, _ io.Reader, _ string, _ int64) error    { return nil }
func (n *NoopStore) Delete(_ context.Context, _ string) error                                  { return nil }
func (n *NoopStore) Enabled() bool                                                             { return false }
