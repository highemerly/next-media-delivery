package variant

import (
	"net/http"
	"testing"
)

func TestParseQueryNoAnim(t *testing.T) {
	tests := []struct {
		query       string
		wantVariant Variant
		wantNoAnim  bool
	}{
		{"emoji=1", Emoji, false},
		{"emoji=1&static=1", Emoji, true},
		{"avatar=1&static=1", Avatar, true},
		{"static=1", Static, true},
		{"preview=1&static=1", Preview, true},
		{"", Raw, false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, "http://x/proxy/a.webp?url=http%3A%2F%2Fo%2Fa.png&"+tt.query, nil)
			got := ParseQuery(r, "")
			if got.Variant != tt.wantVariant {
				t.Errorf("Variant = %v, want %v", got.Variant, tt.wantVariant)
			}
			if got.NoAnim != tt.wantNoAnim {
				t.Errorf("NoAnim = %v, want %v", got.NoAnim, tt.wantNoAnim)
			}
		})
	}
}

// ?emoji=1 and ?emoji=1&static=1 can return different bytes for the same URL,
// so their cache labels must differ. Variants that always flatten keep one label.
func TestCacheString(t *testing.T) {
	tests := []struct {
		variant Variant
		noAnim  bool
		want    string
	}{
		{Emoji, false, "emoji"},
		{Emoji, true, "emoji-static"},
		{Avatar, false, "avatar"},
		{Avatar, true, "avatar-static"},
		{Preview, true, "preview"},
		{Badge, true, "badge"},
		{Static, true, "static"},
		{OGP, true, "ogp"},
		{Raw, true, "raw"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.variant.CacheString(tt.noAnim); got != tt.want {
				t.Errorf("CacheString(%v) = %q, want %q", tt.noAnim, got, tt.want)
			}
		})
	}
}

// AllCacheStrings feeds `purge --all-variants`; a missing or duplicated label
// means cache entries survive a purge.
func TestAllCacheStrings(t *testing.T) {
	all := AllCacheStrings()
	seen := map[string]bool{}
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate label %q", s)
		}
		seen[s] = true
	}
	for _, want := range []string{
		"raw", "emoji", "avatar", "preview", "badge", "static", "ogp",
		"emoji-static", "avatar-static",
	} {
		if !seen[want] {
			t.Errorf("missing label %q", want)
		}
	}
}
