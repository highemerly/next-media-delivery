// Package fallback provides variant-specific fallback images.
// Default images are embedded in the binary; each can be overridden via
// environment variables (file path or base64-encoded data).
package fallback

import (
	"encoding/base64"
	"os"

	"github.com/highemerly/media-delivery/internal/assets"
	"github.com/highemerly/media-delivery/internal/variant"
)

// Image holds fallback image data and its content type.
type Image struct {
	Data        []byte
	ContentType string
}

// Store holds resolved fallback images per variant.
type Store struct {
	avatar  Image
	emoji   Image
	badge   Image
	default_ Image
}

// New loads fallback images from env overrides or embedded defaults.
func New(avatarEnv, emojiEnv, badgeEnv, defaultEnv string) *Store {
	return &Store{
		avatar:   load(avatarEnv, assets.FallbackAvatar, "image/png"),
		emoji:    load(emojiEnv, assets.FallbackEmoji, "image/png"),
		badge:    load(badgeEnv, assets.FallbackBadge, "image/png"),
		default_: load(defaultEnv, assets.FallbackDefault, "image/png"),
	}
}

// Get returns the fallback image for the given variant.
func (s *Store) Get(v variant.Variant) Image {
	switch v {
	case variant.Avatar:
		return s.avatar
	case variant.Emoji:
		return s.emoji
	case variant.Badge:
		return s.badge
	default:
		return s.default_
	}
}

// load resolves an image from an env var value (file path or base64) or falls
// back to the embedded default.
func load(envVal string, embedded []byte, contentType string) Image {
	if envVal == "" {
		return Image{Data: embedded, ContentType: contentType}
	}
	// Try base64 first.
	if data, err := base64.StdEncoding.DecodeString(envVal); err == nil {
		return Image{Data: data, ContentType: contentType}
	}
	// Try file path.
	if data, err := os.ReadFile(envVal); err == nil {
		return Image{Data: data, ContentType: contentType}
	}
	// Fall back to embedded.
	return Image{Data: embedded, ContentType: contentType}
}
