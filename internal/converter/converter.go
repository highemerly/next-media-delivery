// Package converter wraps bimg (libvips) for image transformation.
// The semaphore inside BimgConverter limits concurrent libvips operations
// to CONVERT_CONCURRENCY (default: NumCPU).
package converter

import (
	"context"
	"errors"
	"fmt"

	"github.com/h2non/bimg"
	"github.com/highemerly/media-delivery/internal/format"
	"github.com/highemerly/media-delivery/internal/imageinfo"
	"github.com/highemerly/media-delivery/internal/variant"
)

// Request is an image conversion request.
type Request struct {
	Data    []byte
	Variant variant.Variant
	// Format specifies the desired output format.
	// AVIF is supported only for emoji, avatar, and preview variants;
	// other variants ignore this field and use their fixed output format.
	Format  format.OutputFormat
	// NoAnimation forces a still image even for an animated source.
	// Set from ?static on the emoji/avatar variants; other variants always
	// produce a still image and ignore this field.
	NoAnimation bool
}

// Result is the output of a conversion.
type Result struct {
	Data        []byte
	ContentType string
}

// Converter converts images.
type Converter interface {
	Convert(ctx context.Context, req Request) (*Result, error)
}

// Config holds BimgConverter settings.
type Config struct {
	Concurrency    int
	WebPQuality    int
	PNGCompression int
	AnimQuality    int
	AVIFQuality    int
	AVIFSpeed      int
	// AnimMaxSize caps the size of an animated source that emoji/avatar will
	// pass through unchanged. Larger animations are flattened to a still image
	// instead, so a huge GIF never gets re-served as an avatar.
	AnimMaxSize int64
	// MaxPixels caps width*height of an input that is decoded for
	// conversion, so a small file declaring huge dimensions (a decompression
	// bomb) is rejected before libvips allocates memory for it.
	// 0 disables the check.
	MaxPixels int64
}

// ErrUnsupportedInput is returned when the input is not one of the formats
// accepted for conversion (see allowedInputTypes).
var ErrUnsupportedInput = errors.New("converter: unsupported input format")

// ErrTooManyPixels is returned when the input exceeds Config.MaxPixels.
var ErrTooManyPixels = errors.New("converter: input exceeds the pixel limit")

// allowedInputTypes are the only formats handed to libvips for decoding.
//
// The origin's Content-Type is only checked for an image/ prefix, and bimg
// picks the libvips loader from the magic bytes, so without this list an
// origin could label a PDF, TIFF or any ImageMagick-readable file as
// image/png and have it parsed by poppler, libtiff or ImageMagick (the
// Debian libvips42 package links all of them). Only the formats Misskey
// itself converts are accepted. SVG is kept for custom emoji even though
// libvips classes svgload as untrusted (VIPS_BLOCK_UNTRUSTED would block it).
var allowedInputTypes = map[bimg.ImageType]bool{
	bimg.JPEG: true,
	bimg.PNG:  true,
	bimg.GIF:  true,
	bimg.WEBP: true,
	bimg.HEIF: true,
	bimg.AVIF: true,
	bimg.SVG:  true,
}

// BimgConverter uses libvips via bimg for image processing.
type BimgConverter struct {
	sem  *semaphore
	cfg  Config
}

// New initialises libvips and returns a BimgConverter.
// Caller must call Shutdown() on program exit.
func New(cfg Config) *BimgConverter {
	bimg.Initialize()
	// Disable libvips internal cache — we cache at L1/L2, not inside the library.
	bimg.VipsCacheSetMax(0)
	bimg.VipsCacheSetMaxMem(0)

	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}

	return &BimgConverter{
		sem: newSemaphore(cfg.Concurrency),
		cfg: cfg,
	}
}

// Shutdown releases libvips resources.
func Shutdown() {
	bimg.Shutdown()
}

func (c *BimgConverter) Convert(ctx context.Context, req Request) (*Result, error) {
	if err := c.sem.Acquire(ctx); err != nil {
		return nil, fmt.Errorf("converter: semaphore: %w", err)
	}
	defer c.sem.Release()

	// Variants that serve the original bytes never decode them.
	switch req.Variant {
	case variant.Raw:
		// No conversion: return original data with detected MIME type.
		return passthrough(req.Data), nil
	case variant.Emoji, variant.Avatar:
		if c.keepAnimation(req) {
			return passthrough(req.Data), nil
		}
	}

	if err := c.checkInput(req.Data); err != nil {
		return nil, err
	}

	switch req.Variant {
	case variant.Badge:
		return c.convertPNG(req.Data, 96, 96)

	case variant.Emoji:
		return c.convertToFormat(req.Data, 128, 128, req.Format)

	case variant.Avatar:
		return c.convertToFormat(req.Data, 320, 320, req.Format)

	case variant.Preview:
		return c.convertToFormat(req.Data, 200, 200, req.Format)

	case variant.Static:
		return c.convertStatic(req.Data)

	case variant.OGP:
		return c.convertOGP(req.Data)

	default:
		return nil, fmt.Errorf("converter: unknown variant %v", req.Variant)
	}
}

// checkInput rejects inputs that must not reach a libvips decoder: formats
// outside allowedInputTypes, and images larger than MaxPixels. Reading the
// size only parses the header; the pixels are decoded later by Process.
func (c *BimgConverter) checkInput(data []byte) error {
	t := bimg.DetermineImageType(data)
	if !allowedInputTypes[t] {
		return fmt.Errorf("%w: %s", ErrUnsupportedInput, bimg.ImageTypeName(t))
	}
	if c.cfg.MaxPixels <= 0 {
		return nil
	}
	size, err := bimg.NewImage(data).Size()
	if err != nil {
		return fmt.Errorf("converter: get size: %w", err)
	}
	if px := int64(size.Width) * int64(size.Height); px > c.cfg.MaxPixels {
		return fmt.Errorf("%w: %dx%d", ErrTooManyPixels, size.Width, size.Height)
	}
	return nil
}

// keepAnimation reports whether an emoji/avatar request must be served as the
// unmodified original instead of being converted.
//
// libvips decodes only the first frame of any animated source (bimg never
// passes n=-1 to the loaders, and pngload cannot read APNG frames at all), so
// converting would silently turn an animated custom emoji or avatar into a
// still image. Passing the original through preserves the animation — the same
// thing Misskey's media-proxy does for emoji/avatar. ?static=1 opts out, and
// sources larger than AnimMaxSize fall back to conversion.
func (c *BimgConverter) keepAnimation(req Request) bool {
	if req.NoAnimation {
		return false
	}
	if c.cfg.AnimMaxSize > 0 && int64(len(req.Data)) > c.cfg.AnimMaxSize {
		return false
	}
	return imageinfo.IsAnimated(req.Data)
}

// passthrough returns the input unchanged with its detected MIME type.
func passthrough(data []byte) *Result {
	mime := bimg.DetermineImageTypeName(data)
	if mime == "" {
		return &Result{Data: data, ContentType: "application/octet-stream"}
	}
	return &Result{Data: data, ContentType: "image/" + mime}
}

// convertStatic extracts the first frame of an animated image (GIF/WebP)
// and returns it as a static WebP.
// Strategy: write as WEBP with quality settings; libvips collapses animated
// sources to a single frame when the target format doesn't support animation
// (WEBP save in non-animated mode). For GIF we strip via a first-pass resize.
func (c *BimgConverter) convertStatic(data []byte) (*Result, error) {
	// First convert to PNG (lossless, single frame) then to WebP.
	// This forces libvips to flatten animation.
	intermediate, err := bimg.NewImage(data).Process(bimg.Options{
		Type: bimg.PNG,
	})
	if err != nil {
		// Fallback: try direct WebP conversion.
		return c.convertWebP(data, 0, 0)
	}
	return c.convertWebP(intermediate, 0, 0)
}

// convertToFormat dispatches to convertAVIF or convertWebP based on the requested format.
func (c *BimgConverter) convertToFormat(data []byte, maxW, maxH int, f format.OutputFormat) (*Result, error) {
	if f == format.AVIF {
		return c.convertAVIF(data, maxW, maxH)
	}
	return c.convertWebP(data, maxW, maxH)
}

func (c *BimgConverter) convertAVIF(data []byte, maxW, maxH int) (*Result, error) {
	opts := bimg.Options{
		Type:    bimg.AVIF,
		Quality: c.cfg.AVIFQuality,
		Speed:   c.cfg.AVIFSpeed,
	}

	if maxW > 0 || maxH > 0 {
		img := bimg.NewImage(data)
		size, err := img.Size()
		if err != nil {
			return nil, fmt.Errorf("converter: get size: %w", err)
		}
		opts.Width, opts.Height = fitDimensions(size.Width, size.Height, maxW, maxH)
		opts.Embed = false
	}

	out, err := bimg.NewImage(data).Process(opts)
	if err != nil {
		return nil, fmt.Errorf("converter: avif: %w", err)
	}
	return &Result{Data: out, ContentType: "image/avif"}, nil
}

func (c *BimgConverter) convertWebP(data []byte, maxW, maxH int) (*Result, error) {
	opts := bimg.Options{
		Type:    bimg.WEBP,
		Quality: c.cfg.WebPQuality,
	}

	if maxW > 0 || maxH > 0 {
		img := bimg.NewImage(data)
		size, err := img.Size()
		if err != nil {
			return nil, fmt.Errorf("converter: get size: %w", err)
		}
		opts.Width, opts.Height = fitDimensions(size.Width, size.Height, maxW, maxH)
		opts.Embed = false
	}

	out, err := bimg.NewImage(data).Process(opts)
	if err != nil {
		return nil, fmt.Errorf("converter: webp: %w", err)
	}
	return &Result{Data: out, ContentType: "image/webp"}, nil
}

// OGP card preset limits.
// Width is capped at ogpMaxWidth (X displays cards at ~500–600px, so a 1200px
// source stays crisp without needing more). Height is capped at ogpMaxHeight to
// keep extreme portraits within X's 4096px-per-side limit. Aspect ratio is
// preserved, cropping is never applied, and images smaller than the caps are not
// upscaled (only re-encoded to WebP).
const (
	ogpMaxWidth  = 1200
	ogpMaxHeight = 4096
)

// convertOGP produces an OG card WebP: width ≤ ogpMaxWidth, aspect ratio
// preserved, no crop, no upscale, EXIF/metadata stripped. AVIF/other inputs are
// decoded and re-encoded to WebP so X (Twitter) can render the OG card image.
func (c *BimgConverter) convertOGP(data []byte) (*Result, error) {
	img := bimg.NewImage(data)
	size, err := img.Size()
	if err != nil {
		return nil, fmt.Errorf("converter: get size: %w", err)
	}
	w, h := fitDimensions(size.Width, size.Height, ogpMaxWidth, ogpMaxHeight)
	out, err := bimg.NewImage(data).Process(bimg.Options{
		Type:          bimg.WEBP,
		Quality:       c.cfg.WebPQuality,
		Width:         w,
		Height:        h,
		Embed:         false,
		StripMetadata: true,
	})
	if err != nil {
		return nil, fmt.Errorf("converter: ogp: %w", err)
	}
	return &Result{Data: out, ContentType: "image/webp"}, nil
}

func (c *BimgConverter) convertPNG(data []byte, w, h int) (*Result, error) {
	opts := bimg.Options{
		Type:        bimg.PNG,
		Width:       w,
		Height:      h,
		Compression: c.cfg.PNGCompression,
		Embed:       true,
	}
	out, err := bimg.NewImage(data).Process(opts)
	if err != nil {
		return nil, fmt.Errorf("converter: png: %w", err)
	}
	return &Result{Data: out, ContentType: "image/png"}, nil
}

// fitDimensions scales (srcW, srcH) so that it fits within (maxW, maxH)
// while preserving aspect ratio. Returns the output dimensions.
// If the image already fits, returns the original dimensions.
func fitDimensions(srcW, srcH, maxW, maxH int) (int, int) {
	if maxW == 0 && maxH == 0 {
		return srcW, srcH
	}
	if srcW <= maxW && srcH <= maxH {
		return srcW, srcH
	}
	ratioW := float64(maxW) / float64(srcW)
	ratioH := float64(maxH) / float64(srcH)
	ratio := ratioW
	if ratioH < ratioW {
		ratio = ratioH
	}
	return max(1, int(float64(srcW)*ratio)), max(1, int(float64(srcH)*ratio))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
