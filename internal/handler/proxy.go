// Package handler implements the Misskey-compatible media proxy endpoint.
package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/highemerly/media-delivery/internal/blacklist"
	"github.com/highemerly/media-delivery/internal/cache/l1"
	"github.com/highemerly/media-delivery/internal/cache/l2"
	"github.com/highemerly/media-delivery/internal/cachekey"
	"github.com/highemerly/media-delivery/internal/config"
	"github.com/highemerly/media-delivery/internal/converter"
	"github.com/highemerly/media-delivery/internal/fallback"
	"github.com/highemerly/media-delivery/internal/fetcher"
	"github.com/highemerly/media-delivery/internal/response"
	"github.com/highemerly/media-delivery/internal/store"
	"github.com/highemerly/media-delivery/internal/variant"
)

// sfResult is the value shared among singleflight waiters.
// On origin error, data/contentType are empty and originErr is set.
type sfResult struct {
	data        []byte
	contentType string
	fetchDur    time.Duration
	convertDur  time.Duration
	originErr   error // non-nil when origin returned an error
	// skipCache instructs the caller not to write to L1/L2/AccessTracker.
	// Used when the response should not be cached (e.g. bad Content-Type).
	// Add new conditions here as needed.
	skipCache  bool
	originSize int64 // raw fetch size before conversion; 0 if unknown
}

// Deps holds all dependencies for the proxy handler.
type Deps struct {
	L1        *l1.Cache
	L2        l2.Store
	Tracker   store.AccessTracker
	NegCache  store.NegativeCache
	Circuit   store.CircuitBreaker
	Fetcher   fetcher.Fetcher
	Converter converter.Converter
	Fallback  *fallback.Store
	Blacklist *blacklist.Blacklist
	WG        *sync.WaitGroup
	Cfg       *config.Config
}

// ProxyHandler is the main HTTP handler for /proxy/{filename}.
type ProxyHandler struct {
	deps Deps
	sf   singleflight.Group
}

func NewProxyHandler(deps Deps) *ProxyHandler {
	return &ProxyHandler{deps: deps}
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Parse query parameters.
	parsed := variant.ParseQuery(r)
	v := parsed.Variant
	wantFallback := parsed.WantFallback
	debug := parsed.Debug

	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		http.Error(w, "missing url parameter", http.StatusBadRequest)
		return
	}

	// 2. Validate URL scheme (http/https only).
	parsedURL, err := url.Parse(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		response.WriteError(w, http.StatusForbidden,
			h.deps.Cfg.Cache.ControlDeny,
			"L1=DENY/BAD_REQ",
			"")
		return
	}

	// 3. Compute cache key.
	key := cachekey.Compute(rawURL, v.String())

	// 4. Set Nmd-Cache-Key immediately (always present).
	w.Header().Set("Nmd-Cache-Key", key)

	ctx := r.Context()

	// 5. Negative cache check.
	if negEntry, hit, err := h.deps.NegCache.Get(ctx, key); err == nil && hit {
		xcache := negativeCacheXCache(negEntry.Status)
		if wantFallback && h.deps.Fallback != nil {
			// Serve fallback image without hitting the origin again.
			img := h.deps.Fallback.Get(v)
			response.Write(w, response.Params{
				StatusCode:   http.StatusOK,
				CacheControl: h.deps.Cfg.Cache.ControlFailover,
				ContentType:  img.ContentType,
				Body:         img.Data,
				XCache:       xcache + ", L1=FALLBACK",
				CacheKey:     key,
				LastModified: time.Now(),
				Variant:      v.String(),
			})
			return
		}
		cc, httpStatus := errorCacheControl(h.deps.Cfg, negEntry.Status)
		response.WriteError(w, httpStatus, cc, xcache, key)
		return
	}

	// 6. Circuit breaker check.
	domain := parsedURL.Hostname()
	if allow, err := h.deps.Circuit.Allow(ctx, domain); err == nil && !allow {
		response.WriteError(w, http.StatusServiceUnavailable,
			"max-age=1800",
			"L1=DENY/WAIT",
			key)
		return
	}

	// 7. Blacklist check.
	if h.deps.Blacklist != nil && h.deps.Blacklist.IsDenied(rawURL) {
		response.WriteError(w, http.StatusForbidden,
			h.deps.Cfg.Cache.ControlDeny,
			"L1=DENY/BAD_DOMAIN",
			key)
		return
	}

	// 8. L1 cache lookup (skipped when debug=true).
	xcachePrefix := "L1=MISS"
	if !debug {
		if entry, err := h.deps.L1.Get(key); err == nil && entry != nil {
			h.asyncTrackerSet(key)
			etag := weakETag(entry.StoredAt, entry.Size)
			// RFC 9110 §13.1.3: If-None-Match takes precedence over If-Modified-Since.
			// Evaluate IMS only when INM is absent.
			if checkETagMatch(r, etag) || (r.Header.Get("If-None-Match") == "" && checkNotModified(r, entry.StoredAt)) {
				response.WriteNotModified(w, response.Params{
					CacheControl: h.deps.Cfg.Cache.ControlSuccess,
					XCache:       "L1=HIT",
					CacheKey:     key,
					LastModified: entry.StoredAt,
					ETag:         etag,
					Variant:      v.String(),
				})
				return
			}
			response.Write(w, response.Params{
				StatusCode:   http.StatusOK,
				CacheControl: h.deps.Cfg.Cache.ControlSuccess,
				ContentType:  entry.ContentType,
				Body:         entry.Body,
				XCache:       "L1=HIT",
				CacheKey:     key,
				LastModified: entry.StoredAt,
				ETag:         etag,
				OriginalURL:  rawURL,
				Variant:      v.String(),
			})
			return
		} else if err != nil {
			slog.Error("l1 get error", "key", key, "err", err)
		}

		// 9. L2 cache lookup (S3_ENABLED only; skipped when debug=true).
		if h.deps.L2.Enabled() {
			if obj, err := h.deps.L2.Get(ctx, key); err == nil && obj != nil {
				body, readErr := io.ReadAll(obj.Body)
				obj.Body.Close()
				if readErr == nil {
					h.asyncL1Put(key, body, obj.ContentType)
					h.asyncTrackerSet(key)
					response.Write(w, response.Params{
						StatusCode:   http.StatusOK,
						CacheControl: h.deps.Cfg.Cache.ControlSuccess,
						ContentType:  obj.ContentType,
						Body:         body,
						XCache:       "L1=MISS, L2=HIT",
						CacheKey:     key,
						LastModified: obj.StoredAt,
						OriginalURL:  rawURL,
						Variant:      v.String(),
						OriginalSize: obj.Size,
					})
					return
				}
			}
			xcachePrefix = "L1=MISS, L2=MISS"
		}
	}

	// 10. Origin fetch.
	// debug=true bypasses singleflight to ensure a fresh request every time.
	var sfVal *sfResult
	if debug {
		val, err := h.originFetch(ctx, key, rawURL, v, domain, debug)
		if err != nil {
			h.handleOriginError(ctx, w, key, rawURL, err, xcachePrefix, wantFallback, 0, v)
			return
		}
		sfVal = val
	} else {
		// Singleflight deduplicates concurrent requests for the same key.
		ch := h.sf.DoChan(key, func() (interface{}, error) {
			return h.originFetch(ctx, key, rawURL, v, domain, debug)
		})
		select {
		case res := <-ch:
			if res.Err != nil {
				// Unexpected panic inside singleflight (not an origin error).
				h.handleOriginError(ctx, w, key, rawURL, res.Err, xcachePrefix, wantFallback, 0, v)
				return
			}
			sfVal = res.Val.(*sfResult)
		case <-ctx.Done():
			response.WriteError(w, http.StatusGatewayTimeout,
				h.deps.Cfg.Cache.Control5XX,
				xcachePrefix+", ORI=TIMEOUT",
				key)
			return
		}
	}

	// Origin returned an error (4xx/5xx); fetchDur is still valid.
	if sfVal.originErr != nil {
		h.handleOriginError(ctx, w, key, rawURL, sfVal.originErr, xcachePrefix, wantFallback, sfVal.fetchDur, v)
		return
	}

	// Async cache writes — skipped when skipCache is set (e.g. bad Content-Type).
	if !sfVal.skipCache {
		h.asyncL1Put(key, sfVal.data, sfVal.contentType)
		if h.deps.L2.Enabled() {
			h.asyncL2Put(key, sfVal.data, sfVal.contentType)
		}
		h.asyncTrackerSet(key)
	}

	xcache := xcachePrefix + ", ORI=200"
	now := time.Now()
	response.Write(w, response.Params{
		StatusCode:   http.StatusOK,
		CacheControl: h.deps.Cfg.Cache.ControlSuccess,
		ContentType:  sfVal.contentType,
		Body:         sfVal.data,
		XCache:       xcache,
		CacheKey:     key,
		FetchDur:     sfVal.fetchDur,
		ConvertDur:   sfVal.convertDur,
		LastModified: now,
		ETag:         weakETag(now, int64(len(sfVal.data))),
		OriginalURL:  rawURL,
		Debug:        debug,
		Variant:      v.String(),
		OriginalSize: sfVal.originSize,
	})
}

// originFetch performs the actual fetch + convert pipeline.
// Called inside singleflight, so it runs at most once per key per burst.
func (h *ProxyHandler) originFetch(ctx context.Context, key, rawURL string, v variant.Variant, domain string, debug bool) (*sfResult, error) {
	// Fetch.
	t0 := time.Now()
	result, err := h.deps.Fetcher.Fetch(ctx, rawURL)
	fetchDur := time.Since(t0)
	if err != nil {
		if errors.Is(err, fetcher.ErrFileTooLarge) {
			return &sfResult{fetchDur: fetchDur, originErr: &fileTooLargeError{}}, nil
		}
		h.deps.Circuit.RecordFailure(ctx, domain) //nolint:errcheck
		return &sfResult{fetchDur: fetchDur, originErr: &networkError{err: err, timeout: isTimeoutError(err)}}, nil
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		h.addNegativeCache(ctx, key, result.StatusCode)
		// Circuit Breaker counts only 5xx (server-side failures).
		// 4xx means content is missing, not that the server is down.
		// Network-level failures (DNS, TCP, TLS, timeout) are counted in the err != nil branch above.
		if result.StatusCode >= 500 {
			h.deps.Circuit.RecordFailure(ctx, domain) //nolint:errcheck
		}
		return &sfResult{fetchDur: fetchDur, originErr: &originError{statusCode: result.StatusCode}}, nil
	}

	body, err := io.ReadAll(result.Body)
	result.Body.Close()
	if err != nil {
		return &sfResult{fetchDur: fetchDur, originErr: fmt.Errorf("read body: %w", err)}, nil
	}

	h.deps.Circuit.RecordSuccess(ctx, domain) //nolint:errcheck

	// Content-Type check: reject non-image/video/audio responses unless debug.
	if !debug && !isAllowedContentType(result.ContentType) {
		return &sfResult{
			fetchDur:  fetchDur,
			skipCache: true,
			originErr: &contentTypeError{contentType: result.ContentType},
		}, nil
	}

	// Convert.
	t1 := time.Now()
	var data []byte
	var contentType string
	if v.NeedsConversion() {
		conv, err := h.deps.Converter.Convert(ctx, converter.Request{Data: body, Variant: v})
		if err != nil {
			slog.Error("conversion failed", "variant", v, "err", err)
			return &sfResult{fetchDur: fetchDur, convertDur: time.Since(t1), originErr: fmt.Errorf("convert: %w", err)}, nil
		}
		data = conv.Data
		contentType = conv.ContentType
	} else {
		data = body
		contentType = result.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}
	convertDur := time.Since(t1)

	return &sfResult{
		data:        data,
		contentType: contentType,
		fetchDur:    fetchDur,
		convertDur:  convertDur,
		skipCache:   debug,
		originSize:  result.Size,
	}, nil
}

func (h *ProxyHandler) handleOriginError(ctx context.Context, w http.ResponseWriter, key, rawURL string, err error, xcachePrefix string, wantFallback bool, fetchDur time.Duration, v variant.Variant) {
	// File too large: 413, no fallback, no cache.
	if _, ok := err.(*fileTooLargeError); ok {
		slog.Warn("origin response too large", "url", rawURL)
		response.Write(w, response.Params{
			StatusCode:   http.StatusRequestEntityTooLarge,
			CacheControl: h.deps.Cfg.Cache.Control4XX,
			XCache:       xcachePrefix + ", ORI=200",
			CacheKey:     key,
			FetchDur:     fetchDur,
			LastModified: time.Now(),
		})
		return
	}

	// Network error (timeout, DNS failure, TCP error, etc.).
	if ne, ok := err.(*networkError); ok {
		slog.Error("origin network error", "url", rawURL, "timeout", ne.timeout, "err", ne.err)
		xcacheNet := xcachePrefix + ", ORI=TIMEOUT"
		if wantFallback && h.deps.Fallback != nil {
			img := h.deps.Fallback.Get(v)
			response.Write(w, response.Params{
				StatusCode:   http.StatusOK,
				CacheControl: h.deps.Cfg.Cache.ControlFailover,
				ContentType:  img.ContentType,
				Body:         img.Data,
				XCache:       xcacheNet + ", L1=FALLBACK",
				CacheKey:     key,
				FetchDur:     fetchDur,
				LastModified: time.Now(),
				Variant:      v.String(),
			})
			return
		}
		response.Write(w, response.Params{
			StatusCode:   http.StatusBadGateway,
			CacheControl: h.deps.Cfg.Cache.Control5XX,
			XCache:       xcacheNet,
			CacheKey:     key,
			FetchDur:     fetchDur,
			LastModified: time.Now(),
		})
		return
	}

	xcache := xcachePrefix + ", ORI=ERR"

	// Content-Type rejection: 422 Unprocessable Entity, no fallback.
	// Note: this branch is only reached when debug=false (debug skips the Content-Type check).
	if cte, ok := err.(*contentTypeError); ok {
		slog.Warn("disallowed content-type from origin", "url", rawURL, "content_type", cte.contentType)
		response.Write(w, response.Params{
			StatusCode:   http.StatusUnprocessableEntity,
			CacheControl: h.deps.Cfg.Cache.ControlDeny,
			XCache:       xcachePrefix + ", ORI=200, L1=DENY/BAD_CONTENT",
			CacheKey:     key,
			FetchDur:     fetchDur,
			LastModified: time.Now(),
		})
		return
	}

	var oe *originError
	if isOriginError(err, &oe) {
		cc, httpStatus := errorCacheControl(h.deps.Cfg, oe.statusCode)
		if wantFallback && h.deps.Fallback != nil {
			img := h.deps.Fallback.Get(v)
			response.Write(w, response.Params{
				StatusCode:   http.StatusOK,
				CacheControl: h.deps.Cfg.Cache.ControlFailover,
				ContentType:  img.ContentType,
				Body:         img.Data,
				XCache:       xcache + ", L1=FALLBACK",
				CacheKey:     key,
				FetchDur:     fetchDur,
				LastModified: time.Now(),
				Variant:      v.String(),
			})
			return
		}
		response.Write(w, response.Params{
			StatusCode:   httpStatus,
			CacheControl: cc,
			XCache:       xcache,
			CacheKey:     key,
			FetchDur:     fetchDur,
			LastModified: time.Now(),
		})
		return
	}

	slog.Error("origin fetch failed", "url", rawURL, "err", err)
	response.Write(w, response.Params{
		StatusCode:   http.StatusBadGateway,
		CacheControl: h.deps.Cfg.Cache.Control5XX,
		XCache:       xcache,
		CacheKey:     key,
		FetchDur:     fetchDur,
		LastModified: time.Now(),
	})
}

func (h *ProxyHandler) addNegativeCache(ctx context.Context, key string, status int) {
	ttl := h.deps.Cfg.Security.NegativeCache.TTL5XX
	if status >= 400 && status < 500 {
		ttl = h.deps.Cfg.Security.NegativeCache.TTL40X
	}
	entry := store.NegativeEntry{
		Status:   status,
		ExpireAt: time.Now().Add(ttl),
	}
	if err := h.deps.NegCache.Set(ctx, key, entry); err != nil {
		slog.Error("negative cache set failed", "key", key, "err", err)
	} else {
		slog.Info("negative cache added", "key", key, "status", status, "ttl", ttl)
	}
}

func (h *ProxyHandler) asyncL1Put(key string, data []byte, contentType string) {
	h.deps.WG.Add(1)
	go func() {
		defer h.deps.WG.Done()
		if err := h.deps.L1.Put(key, data, contentType); err != nil {
			slog.Error("l1 write failed", "key", key, "err", err)
		}
	}()
}

func (h *ProxyHandler) asyncL2Put(key string, data []byte, contentType string) {
	h.deps.WG.Add(1)
	go func() {
		defer h.deps.WG.Done()
		if err := h.deps.L2.Put(context.Background(), key, readerFrom(data), contentType, int64(len(data))); err != nil {
			slog.Warn("l2 write failed", "key", key, "err", err)
		}
	}()
}

func (h *ProxyHandler) asyncTrackerSet(key string) {
	h.deps.WG.Add(1)
	go func() {
		defer h.deps.WG.Done()
		if err := h.deps.Tracker.Set(context.Background(), key, time.Now()); err != nil {
			slog.Error("tracker set failed", "key", key, "err", err)
		}
	}()
}

// originError wraps a non-2xx HTTP status from the origin.
type originError struct {
	statusCode int
}

// fileTooLargeError is returned when the origin response exceeds MAX_FILE_SIZE.
type fileTooLargeError struct{}

func (e *fileTooLargeError) Error() string { return "file too large" }

// networkError wraps a fetch-level error (timeout, DNS, TCP, TLS).
type networkError struct {
	err     error
	timeout bool
}

func (e *networkError) Error() string { return e.err.Error() }

// isTimeoutError returns true for context deadline exceeded or HTTP client timeout.
func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Timeout()
	}
	return false
}

// contentTypeError is returned when the origin responds with a disallowed Content-Type.
type contentTypeError struct {
	contentType string
}

func (e *contentTypeError) Error() string {
	return fmt.Sprintf("disallowed content-type: %s", e.contentType)
}

// isAllowedContentType returns true for image/*, video/*, and audio/* MIME types.
func isAllowedContentType(ct string) bool {
	for _, prefix := range []string{"image/", "video/", "audio/"} {
		if len(ct) >= len(prefix) && ct[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func (e *originError) Error() string {
	return fmt.Sprintf("origin returned %d", e.statusCode)
}

func isOriginError(err error, out **originError) bool {
	if oe, ok := err.(*originError); ok {
		*out = oe
		return true
	}
	return false
}

// errorCacheControl returns the appropriate Cache-Control and HTTP status for
// a given origin error status code.
// 410 Gone gets long-TTL immutable (resource is permanently gone).
// Other 4xx get short TTL. 5xx get very short TTL + must-revalidate, mapped to 502.
func errorCacheControl(cfg *config.Config, originStatus int) (cacheControl string, httpStatus int) {
	switch {
	case originStatus == http.StatusGone: // 410
		return cfg.Cache.ControlSuccess, http.StatusGone
	case originStatus >= 500:
		return cfg.Cache.Control5XX, http.StatusBadGateway
	default: // other 4xx
		return cfg.Cache.Control4XX, originStatus
	}
}

func negativeCacheXCache(status int) string {
	if status >= 500 {
		return "L1=HIT/NEGATIVE5XX"
	}
	return "L1=HIT/NEGATIVE4XX"
}

type bytesReader struct{ data []byte; pos int }
func readerFrom(data []byte) io.Reader { return &bytesReader{data: data} }

// weakETag generates a weak ETag value (without the W/"..." wrapper) from
// the file mtime and size. Matches the format used by nginx's FileETag directive.
func weakETag(t time.Time, size int64) string {
	return fmt.Sprintf("%d-%d", t.Unix(), size)
}

// checkETagMatch returns true when the client's If-None-Match value matches
// the given weak ETag, meaning the content has not changed and a 304 should
// be returned. Handles both weak (W/"...") and unquoted forms from clients.
func checkETagMatch(r *http.Request, etag string) bool {
	inm := r.Header.Get("If-None-Match")
	if inm == "" {
		return false
	}
	// Normalise: strip W/" prefix and trailing quote for comparison.
	inm = strings.TrimPrefix(inm, `W/"`)
	inm = strings.TrimPrefix(inm, `"`)
	inm = strings.TrimSuffix(inm, `"`)
	return inm == etag
}

// checkNotModified returns true when the client's If-Modified-Since value is
// >= lastModified, meaning the cached content has not changed since the client
// last fetched it and a 304 Not Modified response should be returned.
// HTTP dates have second precision, so lastModified is truncated before comparison.
func checkNotModified(r *http.Request, lastModified time.Time) bool {
	ims := r.Header.Get("If-Modified-Since")
	if ims == "" {
		return false
	}
	t, err := http.ParseTime(ims)
	if err != nil {
		return false
	}
	return !lastModified.Truncate(time.Second).After(t)
}
func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) { return 0, io.EOF }
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

