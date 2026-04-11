package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/highemerly/media-delivery/internal/store"
)

// contextWithCancel returns a background context.
// Cancellation is handled by the server's graceful shutdown; goroutines
// that need early exit should select on ctx.Done().
func contextWithCancel() context.Context {
	// We use context.Background() here; the goroutines will be cancelled
	// when the process exits after srv.Run() returns.
	return context.Background()
}

func runNegCacheGC(ctx context.Context, negCache store.NegativeCache) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed, err := negCache.GC(ctx)
			if err != nil {
				slog.Error("negative cache gc error", "err", err)
				continue
			}
			if removed > 0 {
				slog.Info("negative cache gc", "removed", removed)
			}
		}
	}
}

func runCircuitBreakerGC(ctx context.Context, cb store.CircuitBreaker) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed, err := cb.GC(ctx)
			if err != nil {
				slog.Error("circuit breaker gc error", "err", err)
				continue
			}
			if removed > 0 {
				slog.Info("circuit breaker gc", "removed", removed)
			}
		}
	}
}
