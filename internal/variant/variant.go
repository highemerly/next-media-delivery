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
	OGP                    // OG card: width ≤1200 WebP, aspect ratio preserved, no crop
)

// String returns the canonical name of the variant.
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
	case OGP:
		return "ogp"
	default:
		return "raw"
	}
}

// CacheString returns the canonical string used in cache keys and in the
// Nmd-Cache-Key `v=` field.
//
// emoji and avatar keep the animation of an animated source (see
// converter.Request.NoAnimation), so `?emoji=1` and `?emoji=1&static=1` can
// produce different bytes for the same URL and must not share a cache entry.
// They are therefore labelled "emoji-static" / "avatar-static". Every other
// variant ignores noAnim because its output is always a still image.
func (v Variant) CacheString(noAnim bool) string {
	if noAnim && v.KeepsAnimation() {
		return v.String() + "-static"
	}
	return v.String()
}

// KeepsAnimation reports whether the variant serves an animated source as-is
// instead of flattening it to a still image.
func (v Variant) KeepsAnimation() bool {
	return v == Emoji || v == Avatar
}

// AllCacheStrings returns every cache-key label the proxy can produce.
// Used by the purge CLI to clear all entries for a URL.
func AllCacheStrings() []string {
	return []string{
		Raw.String(), Emoji.String(), Avatar.String(), Preview.String(),
		Badge.String(), Static.String(), OGP.String(),
		Emoji.CacheString(true), Avatar.CacheString(true),
	}
}

// OutputMIME returns the expected Content-Type after conversion.
// Note: emoji/avatar return the source MIME type instead when the input is
// animated (see KeepsAnimation), so this is only the still-image answer.
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

// SupportsAVIF returns true for variants that support AVIF output.
// Only emoji, avatar, and preview are supported; badge uses PNG, static/raw are not converted to AVIF.
func (v Variant) SupportsAVIF() bool {
	return v == Emoji || v == Avatar || v == Preview
}

// ParseResult is the result of parsing query parameters.
type ParseResult struct {
	Variant      Variant
	WantFallback bool
	Debug        bool // skip Content-Type check (non-Misskey extension)
	// NoAnim is true when ?static is present, even if another variant won the
	// priority contest. Misskey requests `?emoji=1&static=1` when the user has
	// disabled animation playback, so emoji/avatar must be able to see the flag.
	NoAnim bool
}

// ParseQuery derives a Variant and fallback flag from the request query string.
// Priority: emoji > avatar > preview > badge > static > ogp > raw.
// debugKey is compared against the value of the ?debug query parameter;
// Debug is true only when they match and debugKey is non-empty.
func ParseQuery(r *http.Request, debugKey string) ParseResult {
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
	case isSet(q, "ogp"):
		v = OGP
	default:
		v = Raw
	}
	_, wantFallback := q["fallback"]
	debug := debugKey != "" && q.Get("debug") == debugKey
	return ParseResult{
		Variant:      v,
		WantFallback: wantFallback,
		Debug:        debug,
		NoAnim:       isSet(q, "static"),
	}
}

// isSet reports whether the query parameter key is present (regardless of value).
func isSet(q map[string][]string, key string) bool {
	_, ok := q[key]
	return ok
}
