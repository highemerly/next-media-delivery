// Package response centralises all HTTP response header management.
// No origin headers are forwarded; everything is set explicitly.
package response

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const (
	headerCacheControl      = "Cache-Control"
	headerACAllowOrigin     = "Access-Control-Allow-Origin"
	headerCSP               = "Content-Security-Policy"
	headerContentType       = "Content-Type"
	headerContentDisp       = "Content-Disposition"
	headerLastModified      = "Last-Modified"
	headerETag              = "ETag"
	headerXCache            = "Nmd-Cache"
	headerXCacheKey         = "Nmd-Cache-Key"
	headerXCacheable        = "Nmd-Cacheable"
	headerNmdInfo           = "Nmd-Info"
	headerServerTiming      = "Server-Timing"
	headerTimingAllowOrigin = "Timing-Allow-Origin"

	cspValue = "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'"
)

// appVersion is set once at startup via SetVersion.
var appVersion = "dev"

// SetVersion stores the build version to be emitted as Nmd-Info in every response.
func SetVersion(v string) { appVersion = v }

// headersToStrip are removed from the response unconditionally.
var headersToStrip = []string{
	"Age", "Set-Cookie", "Server", "X-Powered-By",
	"Strict-Transport-Security", "Public-Key-Pins",
}

// Params holds everything needed to write a complete response.
type Params struct {
	StatusCode    int
	CacheControl  string
	ContentType   string
	Body          []byte
	XCache        string
	CacheKey      string
	FetchDur      time.Duration
	ConvertDur    time.Duration
	LastModified  time.Time
	ETag          string // weak ETag value, e.g. "1738000000-102400" (without W/" wrapper)
	OriginalURL   string // used to derive Content-Disposition filename
	Debug         bool   // overrides Cache-Control to no-store and adds X-Cacheable: false
	OriginalSize  int64  // size of the origin response before conversion; 0 means unknown (omitted from Nmd-Info)
	Variant       string // request variant (e.g. "avatar", "raw"); always included in Nmd-Info
}

// Write sets all fixed headers and writes the body.
func Write(w http.ResponseWriter, p Params) {
	h := w.Header()

	// Strip forbidden origin headers (in case any were set upstream).
	for _, name := range headersToStrip {
		h.Del(name)
	}

	// Fixed security / CORS headers.
	h.Set(headerACAllowOrigin, "*")
	h.Set(headerCSP, cspValue)
	h.Set(headerTimingAllowOrigin, "*")

	// Cache / diagnostic headers.
	if p.Debug {
		h.Set(headerCacheControl, "no-store")
		h.Set(headerXCacheable, "false")
	} else {
		h.Set(headerCacheControl, p.CacheControl)
	}
	h.Set(headerXCache, p.XCache)
	h.Set(headerXCacheKey, p.CacheKey)
	h.Set(headerNmdInfo, nmdInfo(p.OriginalSize, p.Variant))
	h.Set(headerServerTiming, serverTiming(p.FetchDur, p.ConvertDur))

	// Content headers.
	if p.ContentType != "" {
		h.Set(headerContentType, p.ContentType)
	}
	if p.Body != nil {
		h.Set("Content-Length", fmt.Sprintf("%d", len(p.Body)))
	}
	if !p.LastModified.IsZero() {
		h.Set(headerLastModified, p.LastModified.UTC().Format(http.TimeFormat))
	}
	if p.ETag != "" {
		h.Set(headerETag, `W/"`+p.ETag+`"`)
	}
	if p.OriginalURL != "" {
		h.Set(headerContentDisp, contentDisposition(p.OriginalURL, p.ContentType))
	}

	w.WriteHeader(p.StatusCode)
	if p.Body != nil {
		w.Write(p.Body) //nolint:errcheck
	}
}

// WriteNotModified writes a 304 Not Modified response.
// Per RFC 7232 the body must be empty; only cache/diagnostic headers are set.
func WriteNotModified(w http.ResponseWriter, p Params) {
	h := w.Header()
	h.Set(headerCacheControl, p.CacheControl)
	h.Set(headerXCache, p.XCache)
	h.Set(headerXCacheKey, p.CacheKey)
	h.Set(headerNmdInfo, nmdInfo(p.OriginalSize, p.Variant))
	if !p.LastModified.IsZero() {
		h.Set(headerLastModified, p.LastModified.UTC().Format(http.TimeFormat))
	}
	if p.ETag != "" {
		h.Set(headerETag, `W/"`+p.ETag+`"`)
	}
	w.WriteHeader(http.StatusNotModified)
}

// WriteError writes an error response with appropriate headers.
func WriteError(w http.ResponseWriter, statusCode int, cacheControl, xcache, cacheKey string) {
	Write(w, Params{
		StatusCode:   statusCode,
		CacheControl: cacheControl,
		XCache:       xcache,
		CacheKey:     cacheKey,
		LastModified: time.Now(),
	})
}

func serverTiming(fetch, convert time.Duration) string {
	fetchMS := fetch.Milliseconds()
	convertMS := convert.Milliseconds()
	return fmt.Sprintf("nmdFetch;dur=%d, nmdConvert;dur=%d", fetchMS, convertMS)
}

// nmdInfo builds the Nmd-Info header value.
// originalSize=0 means unknown (e.g. L1 HIT) and is omitted.
func nmdInfo(originalSize int64, variant string) string {
	s := fmt.Sprintf("ver=%s, variant=%s", appVersion, variant)
	if originalSize > 0 {
		s += fmt.Sprintf(", originalSize=%d", originalSize)
	}
	return s
}

// contentDisposition derives a Content-Disposition filename from the origin URL.
// The extension is replaced with the converted type's extension.
func contentDisposition(rawURL, contentType string) string {
	// Extract basename from URL path.
	parts := strings.SplitN(rawURL, "?", 2)
	base := filepath.Base(parts[0])
	if base == "." || base == "/" {
		base = "image"
	}

	// Replace extension based on content type.
	ext := extensionForMIME(contentType)
	if ext != "" {
		base = strings.TrimSuffix(base, filepath.Ext(base)) + ext
	}

	return fmt.Sprintf("inline; filename=%q", base)
}

func extensionForMIME(mime string) string {
	switch {
	case strings.Contains(mime, "webp"):
		return ".webp"
	case strings.Contains(mime, "png"):
		return ".png"
	case strings.Contains(mime, "jpeg"), strings.Contains(mime, "jpg"):
		return ".jpg"
	case strings.Contains(mime, "gif"):
		return ".gif"
	default:
		return ""
	}
}
