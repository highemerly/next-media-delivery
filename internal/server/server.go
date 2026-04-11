// Package server wires up the HTTP server with graceful shutdown.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const shutdownTimeout = 30 * time.Second

// Server holds both the proxy and admin HTTP servers.
type Server struct {
	proxy *http.Server
	admin *http.Server
	// wg tracks in-flight async goroutines (L1/L2 writes, AccessTracker updates).
	wg sync.WaitGroup
}

// Config for building the server.
type Config struct {
	Host      string
	Port      int
	AdminPort int
}

// New creates a Server. Call SetHandlers before Run.
func New(cfg Config) *Server {
	return &Server{
		proxy: &http.Server{
			Addr:        net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port)),
			IdleTimeout: 60 * time.Second,
		},
		admin: &http.Server{
			Addr:        net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", cfg.AdminPort)),
			IdleTimeout: 60 * time.Second,
		},
	}
}

// SetHandlers wires the HTTP handlers after dependencies are built.
func (s *Server) SetHandlers(proxyMux, adminMux http.Handler) {
	s.proxy.Handler = proxyMux
	s.admin.Handler = adminMux
}

// WaitGroup returns the shared WaitGroup for async goroutine tracking.
func (s *Server) WaitGroup() *sync.WaitGroup {
	return &s.wg
}

// Run starts both servers and blocks until SIGTERM/SIGINT, then shuts down gracefully.
func (s *Server) Run() error {
	proxyErrCh := make(chan error, 1)
	adminErrCh := make(chan error, 1)

	go func() {
		slog.Info("proxy server listening", "addr", s.proxy.Addr)
		if err := s.proxy.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			proxyErrCh <- err
		}
	}()
	go func() {
		slog.Info("admin server listening", "addr", s.admin.Addr)
		if err := s.admin.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			adminErrCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-proxyErrCh:
		return fmt.Errorf("proxy server: %w", err)
	case err := <-adminErrCh:
		return fmt.Errorf("admin server: %w", err)
	case sig := <-quit:
		slog.Info("shutdown signal received", "signal", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Stop accepting new connections.
	if err := s.proxy.Shutdown(ctx); err != nil {
		slog.Error("proxy shutdown error", "err", err)
	}
	if err := s.admin.Shutdown(ctx); err != nil {
		slog.Error("admin shutdown error", "err", err)
	}

	// Wait for in-flight async goroutines.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("all async goroutines completed")
	case <-ctx.Done():
		slog.Warn("shutdown timeout: some async goroutines may not have finished")
	}

	return nil
}
