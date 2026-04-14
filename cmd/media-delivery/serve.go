package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/highemerly/media-delivery/internal/assets"
	"github.com/highemerly/media-delivery/internal/response"
	"github.com/highemerly/media-delivery/internal/blacklist"
	"github.com/highemerly/media-delivery/internal/cache/l1"
	"github.com/highemerly/media-delivery/internal/cache/l2"
	"github.com/highemerly/media-delivery/internal/config"
	"github.com/highemerly/media-delivery/internal/converter"
	"github.com/highemerly/media-delivery/internal/fallback"
	"github.com/highemerly/media-delivery/internal/fetcher"
	"github.com/highemerly/media-delivery/internal/handler"
	"github.com/highemerly/media-delivery/internal/middleware"
	"github.com/highemerly/media-delivery/internal/server"
	"github.com/highemerly/media-delivery/internal/store"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the media proxy server (default command)",
		RunE:  runServe,
	}
}

func runServe(_ *cobra.Command, _ []string) error {
	// Automatically set GOMAXPROCS to match k8s CPU limits.
	maxprocs.Set(maxprocs.Logger(func(f string, a ...interface{}) { //nolint:errcheck
		slog.Debug(fmt.Sprintf(f, a...))
	}))

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	response.SetVersion(version)
	if id := os.Getenv("NMD_INSTANCE_ID"); id != "" {
		response.SetInstance(id)
	}
	setupLogger(cfg.Log.Level)

	// L1 disk cache.
	l1Cache := l1.New(cfg.Cache.Dir)

	// L2 object storage (NoopStore until Phase 3).
	var l2Store l2.Store = l2.NewNoopStore()
	// TODO Phase 3: if cfg.S3.Enabled { l2Store = l2.NewS3Store(cfg.S3) }

	// In-memory state stores.
	tracker := store.NewMemoryTracker()
	negCache := store.NewMemoryNegativeCache()
	var circuit store.CircuitBreaker = store.NewNoopCircuitBreaker()
	if cfg.Security.CircuitBreaker.Enabled {
		circuit = store.NewMemoryCircuitBreaker(
			cfg.Security.CircuitBreaker.Threshold,
			cfg.Security.CircuitBreaker.Timeout,
		)
	}

	// HTTP fetcher with SSRF guard.
	fetch := fetcher.New(fetcher.Config{
		Timeout:             cfg.Fetch.Timeout,
		MaxRedirects:        cfg.Fetch.MaxRedirects,
		MaxFileSize:         cfg.Fetch.MaxFileSize,
		AllowedPrivateCIDRs: cfg.Security.AllowedPrivateCIDRs,
		CDNName:             cfg.Server.CDNName,
		Version:             version,
	})

	// Image converter (libvips).
	conv := converter.New(converter.Config{
		Concurrency:    cfg.Convert.Concurrency,
		WebPQuality:    cfg.Convert.WebPQuality,
		PNGCompression: cfg.Convert.PNGCompression,
		AnimQuality:    cfg.Convert.AnimQuality,
		AVIFQuality:    cfg.Convert.AVIFQuality,
		AVIFSpeed:      cfg.Convert.AVIFSpeed,
	})
	defer converter.Shutdown()

	// Build server (WaitGroup is fixed from here).
	srv := server.New(server.Config{
		Host:      cfg.Server.Host,
		Port:      cfg.Server.Port,
		AdminPort: cfg.Admin.Port,
	})

	bl := blacklist.New(
		cfg.Security.BlacklistDomains,
		cfg.Security.BlacklistIPs,
		cfg.Security.BlacklistFile,
	)

	fb := fallback.New(
		cfg.Fallback.Avatar,
		cfg.Fallback.Emoji,
		cfg.Fallback.Badge,
		cfg.Fallback.Default,
	)

	deps := handler.Deps{
		L1:        l1Cache,
		L2:        l2Store,
		Tracker:   tracker,
		NegCache:  negCache,
		Circuit:   circuit,
		Fetcher:   fetch,
		Converter: conv,
		Fallback:  fb,
		Blacklist: bl,
		WG:        srv.WaitGroup(),
		Cfg:       cfg,
	}

	// Proxy mux.
	proxyMux := http.NewServeMux()
	proxyHandler := handler.NewProxyHandler(deps)

	var proxyChain http.Handler = proxyHandler
	proxyChain = middleware.CDNLoop(cfg.Server.CDNName)(proxyChain)
	proxyChain = middleware.Timeout(cfg.Server.RequestTimeout)(proxyChain)
	proxyChain = middleware.Recovery(proxyChain)

	proxyMux.Handle("/proxy/", proxyChain)
	proxyMux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(assets.RobotsTxt) //nolint:errcheck
	})
	proxyMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Admin mux (localhost only, not exposed externally).
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/stats", makeStatsHandler(l1Cache, tracker, negCache, circuit, cfg))
	adminMux.HandleFunc("/stats/circuit-breaker", makeCircuitBreakerStatsHandler(circuit))
	adminMux.HandleFunc("/stats/negative-cache", makeNegativeCacheStatsHandler(negCache))
	adminMux.HandleFunc("/cache", makePurgeAllHandler(l1Cache, l2Store, tracker))
	adminMux.HandleFunc("/cache/", makePurgeHandler(l1Cache, l2Store))

	srv.SetHandlers(proxyMux, adminMux)

	// Start background goroutines.
	ctx := contextWithCancel()
	go l1.NewCleaner(l1Cache, tracker,
		cfg.Cache.MaxBytes, cfg.Cache.TargetBytes,
		5*time.Minute,
	).Run(ctx)
	go runNegCacheGC(ctx, negCache)
	if cfg.Security.CircuitBreaker.Enabled {
		go runCircuitBreakerGC(ctx, circuit)
	}

	return srv.Run()
}

func setupLogger(level string) {
	var l slog.Level
	switch level {
	case "DEBUG":
		l = slog.LevelDebug
	case "WARN":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
