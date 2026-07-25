package imageinfo

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestIsAnimated(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"APNG", buildAPNG(3), true},
		{"APNG single frame", buildAPNG(1), true}, // acTL present ⇒ animation container
		{"still PNG", buildPNG(t), false},
		{"animated GIF", buildGIF(t, 3), true},
		{"still GIF", buildGIF(t, 1), false},
		{"animated WebP", buildAnimatedWebP(), true},
		{"still WebP", buildStillWebP(), false},
		{"JPEG", buildJPEG(t), false},
		{"empty", nil, false},
		{"garbage", []byte("not an image at all"), false},
		{"truncated APNG", buildAPNG(3)[:20], false},
		{"truncated GIF", buildGIF(t, 3)[:20], false},
		{"PNG signature only", pngSignature, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAnimated(tt.data); got != tt.want {
				t.Errorf("IsAnimated() = %v, want %v", got, tt.want)
			}
		})
	}
}

// buildAPNG assembles a minimal APNG: IHDR, acTL, fcTL+IDAT for the first
// frame, then fcTL+fdAT pairs for the rest.
func buildAPNG(frames int) []byte {
	const w, h = 4, 4

	chunk := func(typ string, data []byte) []byte {
		body := append([]byte(typ), data...)
		out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
		out = append(out, body...)
		return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(body))
	}
	fcTL := func(seq uint32) []byte {
		b := binary.BigEndian.AppendUint32(nil, seq)
		b = binary.BigEndian.AppendUint32(b, w)
		b = binary.BigEndian.AppendUint32(b, h)
		b = binary.BigEndian.AppendUint32(b, 0) // x offset
		b = binary.BigEndian.AppendUint32(b, 0) // y offset
		b = binary.BigEndian.AppendUint16(b, 1) // delay numerator
		b = binary.BigEndian.AppendUint16(b, 10)
		return append(b, 0, 0) // dispose / blend op
	}

	ihdr := binary.BigEndian.AppendUint32(nil, w)
	ihdr = binary.BigEndian.AppendUint32(ihdr, h)
	ihdr = append(ihdr, 8, 2, 0, 0, 0) // 8bit, truecolour

	out := append([]byte{}, pngSignature...)
	out = append(out, chunk("IHDR", ihdr)...)
	out = append(out, chunk("acTL", binary.BigEndian.AppendUint32(
		binary.BigEndian.AppendUint32(nil, uint32(frames)), 0))...)

	var seq uint32
	out = append(out, chunk("fcTL", fcTL(seq))...)
	seq++
	out = append(out, chunk("IDAT", []byte{0x78, 0x9c, 0x01, 0x00, 0x00, 0xff, 0xff})...)
	for i := 1; i < frames; i++ {
		out = append(out, chunk("fcTL", fcTL(seq))...)
		seq++
		out = append(out, chunk("fdAT", binary.BigEndian.AppendUint32(nil, seq))...)
		seq++
	}
	return append(out, chunk("IEND", nil)...)
}

func buildPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func buildGIF(t *testing.T, frames int) []byte {
	t.Helper()
	g := &gif.GIF{}
	for i := 0; i < frames; i++ {
		img := image.NewPaletted(image.Rect(0, 0, 4, 4), palette.Plan9)
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

func buildJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// riff wraps chunks in a RIFF/WEBP container. Payloads are padded to an even
// length, exactly as the WebP container spec requires.
func riff(chunks ...[]byte) []byte {
	body := []byte("WEBP")
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
	return append(out, body...)
}

func webpChunk(typ string, payload []byte) []byte {
	out := append([]byte(typ), binary.LittleEndian.AppendUint32(nil, uint32(len(payload)))...)
	out = append(out, payload...)
	if len(payload)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func buildAnimatedWebP() []byte {
	vp8x := make([]byte, 10)
	vp8x[0] = 0x02 // animation flag
	return riff(
		webpChunk("VP8X", vp8x),
		webpChunk("ANIM", make([]byte, 6)),
		webpChunk("ANMF", make([]byte, 17)), // odd length ⇒ exercises padding
	)
}

func buildStillWebP() []byte {
	return riff(webpChunk("VP8 ", make([]byte, 23)))
}
