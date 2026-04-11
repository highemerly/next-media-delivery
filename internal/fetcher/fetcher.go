// Package fetcher provides an HTTP client for fetching origin media.
// SSRF protection is enforced by ssrf.go via a custom DialContext.
package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrFileTooLarge is returned when the response body exceeds MaxFileSize.
var ErrFileTooLarge = errors.New("file too large")

const userAgentBase = "NextMediaDelivery/1.0 (+https://github.com/highemerly/media-delivery; misskey compatible media proxy)"

// Result holds a successfully fetched response.
type Result struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64 // -1 if unknown
	StatusCode  int
}

// Fetcher fetches a URL and returns the raw result.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (*Result, error)
}

// Config holds HTTPFetcher configuration.
type Config struct {
	Timeout             time.Duration
	MaxRedirects        int
	MaxFileSize         int64
	AllowedPrivateCIDRs []string // empty = block all private
	CDNName             string   // used for outbound CDN-Loop header (RFC 8586)
}

// HTTPFetcher is a production Fetcher with redirect limit and SSRF guard.
type HTTPFetcher struct {
	client      *http.Client
	maxFileSize int64
	cdnName     string
	userAgent   string
}

func New(cfg Config) *HTTPFetcher {
	transport := newSSRFTransport(cfg.AllowedPrivateCIDRs)
	// Disable automatic Accept-Encoding / transparent decompression.
	transport.base.DisableCompression = true

	redirectsLeft := cfg.MaxRedirects
	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= redirectsLeft {
				return fmt.Errorf("stopped after %d redirects", redirectsLeft)
			}
			// Sanitize redirect request: remove sensitive headers.
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
			return nil
		},
	}
	ua := userAgentBase
	if cfg.CDNName != "" {
		ua = userAgentBase[:len(userAgentBase)-1] + "; instance=" + cfg.CDNName + ")"
	}
	return &HTTPFetcher{client: client, maxFileSize: cfg.MaxFileSize, cdnName: cfg.CDNName, userAgent: ua}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, rawURL string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "image/*, video/*, audio/*, */*;q=0.8")
	if f.cdnName != "" {
		cdnInfo := f.cdnName + "; v=1.0"
		existing := req.Header.Get("CDN-Loop")
		if existing == "" {
			req.Header.Set("CDN-Loop", cdnInfo)
		} else {
			req.Header.Set("CDN-Loop", existing+", "+cdnInfo)
		}
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 600 {
		resp.Body.Close()
		return &Result{StatusCode: resp.StatusCode}, nil
	}

	// Limit body size and detect oversize responses.
	limited := io.LimitReader(resp.Body, f.maxFileSize+1)
	body, err := io.ReadAll(limited)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > f.maxFileSize {
		return nil, ErrFileTooLarge
	}

	return &Result{
		Body:        io.NopCloser(io.NopCloser(newBytesReader(body))),
		ContentType: resp.Header.Get("Content-Type"),
		Size:        int64(len(body)),
		StatusCode:  resp.StatusCode,
	}, nil
}

type bytesReader struct{ data []byte; pos int }
func newBytesReader(data []byte) *bytesReader { return &bytesReader{data: data} }
func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) { return 0, io.EOF }
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
