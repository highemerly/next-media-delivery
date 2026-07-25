// Package imageinfo inspects raw image bytes without decoding them.
//
// It exists because libvips (via bimg) cannot tell us whether an input is
// animated: bimg never passes n=-1 to the loaders, and pngload has no page
// support at all, so every decode yields a single frame regardless of the
// source. Detection therefore has to happen on the raw container bytes.
package imageinfo

import "bytes"

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// IsAnimated reports whether data is a multi-frame image: APNG, animated GIF,
// or animated WebP. Any other input (including malformed or truncated data)
// reports false, so callers fall back to the normal conversion path.
func IsAnimated(data []byte) bool {
	switch {
	case bytes.HasPrefix(data, pngSignature):
		return isAnimatedPNG(data)
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return isAnimatedGIF(data)
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return isAnimatedWebP(data)
	}
	return false
}

// isAnimatedPNG walks the PNG chunk list looking for the APNG animation
// control chunk (acTL). Per the APNG spec acTL must precede the first IDAT,
// so reaching IDAT means the file is a plain still PNG.
func isAnimatedPNG(data []byte) bool {
	p := len(pngSignature)
	for p+8 <= len(data) {
		length := int(be32(data[p:]))
		typ := data[p+4 : p+8]
		switch {
		case bytes.Equal(typ, []byte("acTL")):
			return true
		case bytes.Equal(typ, []byte("IDAT")):
			return false
		}
		// length + type + data + crc; guard against overflow on corrupt input.
		if length < 0 {
			return false
		}
		p += 12 + length
		if p <= 0 {
			return false
		}
	}
	return false
}

// isAnimatedWebP walks the RIFF chunk list looking for the ANIM chunk, which
// is present only in animated (VP8X extended) files.
func isAnimatedWebP(data []byte) bool {
	p := 12 // "RIFF" + size + "WEBP"
	for p+8 <= len(data) {
		typ := data[p : p+4]
		size := int(le32(data[p+4:]))
		if bytes.Equal(typ, []byte("ANIM")) {
			return true
		}
		if size < 0 {
			return false
		}
		// Chunk payloads are padded to an even length.
		p += 8 + size + size%2
		if p <= 0 {
			return false
		}
	}
	return false
}

// isAnimatedGIF walks the GIF block stream and reports true as soon as a
// second image descriptor is found. A single-frame GIF (the common case for
// still images saved as GIF) reports false.
func isAnimatedGIF(data []byte) bool {
	// Logical screen descriptor: 6-byte header + 7 bytes, the 11th byte
	// (index 10) holding the global colour table flag and size.
	if len(data) < 13 {
		return false
	}
	p := 13
	if data[10]&0x80 != 0 {
		p += 3 << ((data[10] & 0x07) + 1)
	}

	frames := 0
	for p < len(data) {
		switch data[p] {
		case 0x21: // extension: introducer + label, then sub-blocks
			p += 2
			var ok bool
			if p, ok = skipSubBlocks(data, p); !ok {
				return false
			}
		case 0x2C: // image descriptor: introducer + 8 bytes + packed field
			frames++
			if frames >= 2 {
				return true
			}
			if p+10 > len(data) {
				return false
			}
			packed := data[p+9]
			p += 10
			if packed&0x80 != 0 { // local colour table
				p += 3 << ((packed & 0x07) + 1)
			}
			p++ // LZW minimum code size
			var ok bool
			if p, ok = skipSubBlocks(data, p); !ok {
				return false
			}
		case 0x3B: // trailer
			return false
		default: // unexpected byte: give up rather than guess
			return false
		}
		if p <= 0 {
			return false
		}
	}
	return false
}

// skipSubBlocks advances past a GIF sub-block chain (length-prefixed blocks
// terminated by a zero-length block). ok is false when the data is truncated.
func skipSubBlocks(data []byte, p int) (int, bool) {
	for p < len(data) {
		n := int(data[p])
		p++
		if n == 0 {
			return p, true
		}
		p += n
	}
	return p, false
}

func be32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
