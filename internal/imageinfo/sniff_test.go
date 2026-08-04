package imageinfo

import "testing"

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"PNG", buildPNG(t), "image/png"},
		{"APNG", buildAPNG(3), "image/png"},
		{"GIF", buildGIF(t, 1), "image/gif"},
		{"animated GIF", buildGIF(t, 3), "image/gif"},
		{"JPEG", buildJPEG(t), "image/jpeg"},
		{"WebP", buildStillWebP(), "image/webp"},
		{"animated WebP", buildAnimatedWebP(), "image/webp"},
		{"AVIF", buildISOBMFF("avif"), "image/avif"},
		{"AVIF sequence", buildISOBMFF("avis"), "image/avif"},
		{"HEIC", buildISOBMFF("heic"), "image/heic"},
		{"HEIF", buildISOBMFF("mif1"), "image/heif"},
		{"MP4", buildISOBMFF("mp42"), "video/mp4"},

		// Non-media bodies must stay rejected: these are exactly the responses
		// (HTML error pages, JSON APIs) the Content-Type check exists to block.
		{"HTML", []byte("<!DOCTYPE html><html><body>404</body></html>"), ""},
		{"JSON", []byte(`{"error":"not found"}`), ""},
		{"plain text", []byte("just some text"), ""},
		{"empty", nil, ""},
		{"garbage", []byte("not an image at all"), ""},
		{"ftyp too short", []byte("\x00\x00\x00\x18ftypav"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectContentType(tt.data); got != tt.want {
				t.Errorf("DetectContentType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// buildISOBMFF returns a minimal ISO-BMFF header with the given major brand.
// The ftyp box is 24 bytes: size + "ftyp" + major brand + minor version +
// two compatible brands, which is enough for both our brand check and the
// net/http MP4 sniffer (it scans the compatible brands for "mp4").
func buildISOBMFF(brand string) []byte {
	box := []byte{0x00, 0x00, 0x00, 0x18}
	box = append(box, "ftyp"...)
	box = append(box, brand...)
	box = append(box, 0x00, 0x00, 0x00, 0x00) // minor version
	box = append(box, brand...)
	box = append(box, "mif1"...)
	return box
}
