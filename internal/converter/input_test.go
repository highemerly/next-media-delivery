package converter

import (
	"context"
	"encoding/binary"
	"errors"
	"strconv"
	"testing"

	"github.com/highemerly/media-delivery/internal/format"
	"github.com/highemerly/media-delivery/internal/variant"
)

// minimalPDF is enough for bimg's magic-byte check ("%PDF") to pick pdfload.
var minimalPDF = []byte("%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
	"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
	"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 8 8]>>endobj\n" +
	"trailer<</Root 1 0 R>>\n%%EOF\n")

func svg(w, h int) []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="` + strconv.Itoa(w) + `" height="` + strconv.Itoa(h) +
		`"><rect width="100%" height="100%" fill="red"/></svg>`)
}

// bmp1x1 builds a 1x1 24-bit BMP. libvips has no native BMP loader, so bimg
// reports it as MAGICK (decoded through ImageMagick).
func bmp1x1() []byte {
	b := make([]byte, 58)
	copy(b, "BM")
	binary.LittleEndian.PutUint32(b[2:], 58)  // file size
	binary.LittleEndian.PutUint32(b[10:], 54) // pixel data offset
	binary.LittleEndian.PutUint32(b[14:], 40) // BITMAPINFOHEADER
	binary.LittleEndian.PutUint32(b[18:], 1)  // width
	binary.LittleEndian.PutUint32(b[22:], 1)  // height
	binary.LittleEndian.PutUint16(b[26:], 1)  // planes
	binary.LittleEndian.PutUint16(b[28:], 24) // bpp
	binary.LittleEndian.PutUint32(b[34:], 4)  // image size (row padded to 4)
	b[54], b[55], b[56] = 0, 0, 255
	return b
}

func newInputTestConverter(maxPixels int64) *BimgConverter {
	return New(Config{
		Concurrency:    1,
		WebPQuality:    80,
		PNGCompression: 6,
		AVIFQuality:    60,
		AVIFSpeed:      6,
		MaxPixels:      maxPixels,
	})
}

// Only the allowed formats may reach a libvips decoder, and the pixel limit
// applies before decoding. Raw passthrough never decodes, so it is unaffected.
func TestConvertInputCheck(t *testing.T) {
	c := newInputTestConverter(100_000_000)
	small := newInputTestConverter(99) // test-small.png is 10x10 = 100 px

	tests := []struct {
		name    string
		conv    *BimgConverter
		data    []byte
		variant variant.Variant
		wantErr error // nil = conversion must succeed
	}{
		{"PDF rejected", c, minimalPDF, variant.Preview, ErrUnsupportedInput},
		{"ImageMagick (BMP) rejected", c, bmp1x1(), variant.Emoji, ErrUnsupportedInput},
		{"unknown bytes rejected", c, []byte("this is not an image at all"), variant.Avatar, ErrUnsupportedInput},
		{"SVG converted", c, svg(40, 30), variant.Emoji, nil},
		{"SVG over the pixel limit rejected", c, svg(20000, 20000), variant.Emoji, ErrTooManyPixels},
		{"PNG over the pixel limit rejected", small, readAsset(t, "test-small.png"), variant.Badge, ErrTooManyPixels},
		{"PNG converted", c, readAsset(t, "test-small.png"), variant.Static, nil},
		{"raw passes PDF through untouched", c, minimalPDF, variant.Raw, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.conv.Convert(context.Background(), Request{
				Data:    tt.data,
				Variant: tt.variant,
				Format:  format.WebP,
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			if len(res.Data) == 0 {
				t.Fatal("empty output")
			}
		})
	}
}
