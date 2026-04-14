// Package format defines the output image format derived from the request filename extension.
package format

import (
	"path/filepath"
	"strings"
)

// OutputFormat represents the desired output image format.
type OutputFormat int

const (
	WebP OutputFormat = iota // default; also used as fallback
	AVIF
)

// String returns the canonical string used in cache keys.
func (f OutputFormat) String() string {
	if f == AVIF {
		return "avif"
	}
	return "webp"
}

// ContentType returns the MIME type for the output format.
func (f OutputFormat) ContentType() string {
	if f == AVIF {
		return "image/avif"
	}
	return "image/webp"
}

// FromFilename derives an OutputFormat from the extension of a filename.
// Only ".avif" maps to AVIF; everything else (including no extension) maps to WebP.
func FromFilename(filename string) OutputFormat {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".avif" {
		return AVIF
	}
	return WebP
}
