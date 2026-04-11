// Package assets embeds static files into the binary.
package assets

import _ "embed"

//go:embed robots.txt
var RobotsTxt []byte

//go:embed fallback_avatar.png
var FallbackAvatar []byte

//go:embed fallback_emoji.png
var FallbackEmoji []byte

//go:embed fallback_badge.png
var FallbackBadge []byte

//go:embed fallback_default.png
var FallbackDefault []byte
