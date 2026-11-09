package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/highemerly/media-delivery/internal/cache/l1"
	"github.com/highemerly/media-delivery/internal/cache/l2"
	"github.com/highemerly/media-delivery/internal/cachekey"
	"github.com/highemerly/media-delivery/internal/config"
	"github.com/highemerly/media-delivery/internal/fetcher"
	"github.com/highemerly/media-delivery/internal/store"
)

// blockingFetcher holds every Fetch until release is closed, so a test can
// cancel the calling request while the origin fetch is still in flight.
type blockingFetcher struct {
	started chan struct{}
	release chan struct{}
	body    []byte

	mu       sync.Mutex
	calls    int
	ctxErr   error // ctx.Err() observed when the fetch finished
}

func (f *blockingFetcher) Fetch(ctx context.Context, _ string) (*fetcher.Result, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	f.started <- struct{}{}

	select {
	case <-f.release:
	case <-ctx.Done():
	}
	f.mu.Lock()
	f.ctxErr = ctx.Err()
	f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &fetcher.Result{
		Body:        io.NopCloser(bytes.NewReader(f.body)),
		ContentType: "image/png",
		Size:        int64(len(f.body)),
		StatusCode:  http.StatusOK,
	}, nil
}

func newTestHandler(t *testing.T, f fetcher.Fetcher) (*ProxyHandler, *l1.Cache, *store.MemoryCircuitBreaker, *sync.WaitGroup) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server.RequestTimeout = 10 * time.Second
	cfg.Cache.ControlSuccess = "max-age=31536000, immutable"
	cfg.Cache.Control5XX = "max-age=120, must-revalidate"
	cfg.Cache.AsyncWriteTimeout = 5 * time.Second

	cache := l1.New(t.TempDir())
	// Threshold 1: a single counted failure would open the circuit.
	circuit := store.NewMemoryCircuitBreaker(1, time.Minute)
	wg := &sync.WaitGroup{}
	h := NewProxyHandler(Deps{
		L1:          cache,
		L2:          l2.NewNoopStore(),
		Tracker:     store.NewMemoryTracker(),
		NegCache:    store.NewMemoryNegativeCache(),
		Circuit:     circuit,
		Fetcher:     f,
		WG:          wg,
		Cfg:         cfg,
		ShutdownCtx: context.Background(),
	})
	return h, cache, circuit, wg
}

// A client that disconnects while the origin fetch is in flight must not
// cancel the shared fetch, must not be counted as an origin failure by the
// circuit breaker, and the fetched result must still be cached.
func TestClientCancelDoesNotAbortSharedFetch(t *testing.T) {
	f := &blockingFetcher{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		body:    []byte("\x89PNG\r\n\x1a\nfake"),
	}
	h, cache, circuit, wg := newTestHandler(t, f)

	origin := "http://origin.example/a.png"
	target := "/proxy/a.png?url=" + url.QueryEscape(origin)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	<-f.started
	cancel() // client disconnects
	<-done
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("cancelled caller: status = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}

	close(f.release) // origin answers after the caller left
	// Wait for the shared fetch to finish (it holds the singleflight key) and
	// for its async cache writes.
	deadline := time.Now().Add(5 * time.Second)
	key := cachekey.Compute(origin, "raw", "webp")
	for {
		wg.Wait()
		if e, _ := cache.Get(key); e != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("result was not written to L1 after the caller disconnected")
		}
		time.Sleep(10 * time.Millisecond)
	}

	f.mu.Lock()
	ctxErr, calls := f.ctxErr, f.calls
	f.mu.Unlock()
	if ctxErr != nil {
		t.Errorf("shared fetch context was cancelled by the client: %v", ctxErr)
	}
	if calls != 1 {
		t.Errorf("fetch calls = %d, want 1", calls)
	}
	snap, _ := circuit.Snapshot(context.Background())
	if len(snap) != 0 {
		t.Errorf("circuit breaker recorded the client cancel: %+v", snap)
	}

	// The next request is served from L1 without another origin fetch.
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, target, nil))
	if rec2.Code != http.StatusOK || rec2.Header().Get("Nmd-Cache") != "L1=HIT" {
		t.Errorf("second request: status = %d, Nmd-Cache = %q, want 200 L1=HIT", rec2.Code, rec2.Header().Get("Nmd-Cache"))
	}
}
