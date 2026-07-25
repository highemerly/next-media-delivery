package converter

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color/palette"
	"image/gif"
	"os"
	"testing"

	"github.com/highemerly/media-delivery/internal/format"
	"github.com/highemerly/media-delivery/internal/variant"
)

// Animated sources must survive emoji/avatar untouched, because libvips can
// only ever decode their first frame. Everything else stays a still image.
func TestConvertAnimated(t *testing.T) {
	c := newTestConverter(0)
	apng := buildAPNG(t)
	agif := buildAnimatedGIF(t)
	still := readAsset(t, "test-small.png")

	tests := []struct {
		name        string
		data        []byte
		variant     variant.Variant
		noAnimation bool
		wantCT      string
		wantSame    bool // response body is the original, byte for byte
	}{
		{"emoji keeps APNG", apng, variant.Emoji, false, "image/png", true},
		{"avatar keeps APNG", apng, variant.Avatar, false, "image/png", true},
		{"emoji keeps animated GIF", agif, variant.Emoji, false, "image/gif", true},
		{"avatar keeps animated GIF", agif, variant.Avatar, false, "image/gif", true},
		{"emoji + static flattens APNG", apng, variant.Emoji, true, "image/webp", false},
		{"avatar + static flattens GIF", agif, variant.Avatar, true, "image/webp", false},
		{"preview flattens APNG", apng, variant.Preview, false, "image/webp", false},
		{"static flattens APNG", apng, variant.Static, false, "image/webp", false},
		{"badge flattens APNG", apng, variant.Badge, false, "image/png", false},
		{"ogp flattens APNG", apng, variant.OGP, false, "image/webp", false},
		{"emoji converts still PNG", still, variant.Emoji, false, "image/webp", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := c.Convert(context.Background(), Request{
				Data:        tt.data,
				Variant:     tt.variant,
				Format:      format.WebP,
				NoAnimation: tt.noAnimation,
			})
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			if res.ContentType != tt.wantCT {
				t.Errorf("ContentType = %q, want %q", res.ContentType, tt.wantCT)
			}
			if same := bytes.Equal(res.Data, tt.data); same != tt.wantSame {
				t.Errorf("body identical to source = %v, want %v", same, tt.wantSame)
			}
		})
	}
}

// An AVIF request for an animated source still passes the original through:
// preserving the animation matters more than honouring the extension.
func TestConvertAnimatedAVIFRequest(t *testing.T) {
	c := newTestConverter(0)
	res, err := c.Convert(context.Background(), Request{
		Data: buildAPNG(t), Variant: variant.Emoji, Format: format.AVIF,
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", res.ContentType)
	}
}

// Animations larger than AnimMaxSize are flattened instead of passed through.
func TestConvertAnimatedOverMaxSize(t *testing.T) {
	apng := buildAPNG(t)
	c := newTestConverter(int64(len(apng)) - 1)
	res, err := c.Convert(context.Background(), Request{
		Data: apng, Variant: variant.Emoji, Format: format.WebP,
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.ContentType != "image/webp" {
		t.Errorf("ContentType = %q, want image/webp", res.ContentType)
	}
}

func newTestConverter(animMaxSize int64) *BimgConverter {
	return New(Config{
		Concurrency:    1,
		WebPQuality:    80,
		PNGCompression: 6,
		AVIFQuality:    60,
		AVIFSpeed:      6,
		AnimMaxSize:    animMaxSize,
	})
}

func readAsset(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../assets/" + name)
	if err != nil {
		t.Fatalf("read asset %s: %v", name, err)
	}
	return data
}

// buildAPNG turns a real PNG asset into an APNG by splicing an acTL chunk in
// front of its first IDAT. The pixel data stays valid, so libvips can still
// decode it as a still image when ?static=1 asks it to.
func buildAPNG(t *testing.T) []byte {
	t.Helper()
	src := readAsset(t, "test-small.png")
	idat := bytes.Index(src, []byte("IDAT"))
	if idat < 4 {
		t.Fatal("no IDAT chunk in test asset")
	}
	acTLData := binary.BigEndian.AppendUint32(nil, 2) // 2 frames
	acTLData = binary.BigEndian.AppendUint32(acTLData, 0)
	body := append([]byte("acTL"), acTLData...)
	acTL := binary.BigEndian.AppendUint32(nil, uint32(len(acTLData)))
	acTL = append(acTL, body...)
	acTL = binary.BigEndian.AppendUint32(acTL, crc32.ChecksumIEEE(body))

	out := append([]byte{}, src[:idat-4]...) // insert before the IDAT length field
	out = append(out, acTL...)
	return append(out, src[idat-4:]...)
}

func buildAnimatedGIF(t *testing.T) []byte {
	t.Helper()
	g := &gif.GIF{}
	for i := 0; i < 3; i++ {
		img := image.NewPaletted(image.Rect(0, 0, 32, 32), palette.Plan9)
		img.SetColorIndex(0, 0, uint8(i))
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
