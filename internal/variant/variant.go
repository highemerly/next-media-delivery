package variant

import "net/http"

// Variant represents the image transformation mode derived from query parameters.
type Variant int

const (
	Raw     Variant = iota // no transformation, return original
	Emoji                  // ≤128×128 WebP
	Avatar                 // ≤320×320 WebP
	Preview                // ≤200×200 WebP
	Badge                  // 96×96 PNG
	Static                 // first frame only WebP (for animated images)
)

// String returns the canonical string used in cache keys.
func (v Variant) String() string {
	switch v {
	case Emoji:
		return "emoji"
	case Avatar:
		return "avatar"
	case Preview:
		return "preview"
	case Badge:
		return "badge"
	case Static:
		return "static"
	default:
		return "raw"
	}
}

// OutputMIME returns the expected Content-Type after conversion.
func (v Variant) OutputMIME() string {
	if v == Badge {
		return "image/png"
	}
	return "image/webp"
}

// NeedsConversion returns false for Raw variant (pass-through).
func (v Variant) NeedsConversion() bool {
	return v != Raw
}

// ParseResult is the result of parsing query parameters.
type ParseResult struct {
	Variant      Variant
	WantFallback bool
	Debug        bool // skip Content-Type check (non-Misskey extension)
}

// ParseQuery derives a Variant and fallback flag from the request query string.
// Priority: emoji > avatar > preview > badge > static > raw.
func ParseQuery(r *http.Request) ParseResult {
	q := r.URL.Query()
	var v Variant
	switch {
	case isSet(q, "emoji"):
		v = Emoji
	case isSet(q, "avatar"):
		v = Avatar
	case isSet(q, "preview"):
		v = Preview
	case isSet(q, "badge"):
		v = Badge
	case isSet(q, "static"):
		v = Static
	default:
		v = Raw
	}
	_, wantFallback := q["fallback"]
	_, debug := q["debug"]
	return ParseResult{Variant: v, WantFallback: wantFallback, Debug: debug}
}

// isSet reports whether the query parameter key is present (regardless of value).
func isSet(q map[string][]string, key string) bool {
	_, ok := q[key]
	return ok
}
