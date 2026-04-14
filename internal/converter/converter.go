// Package converter wraps bimg (libvips) for image transformation.
// The semaphore inside BimgConverter limits concurrent libvips operations
// to CONVERT_CONCURRENCY (default: NumCPU).
package converter

import (
	"context"
	"fmt"

	"github.com/h2non/bimg"
	"github.com/highemerly/media-delivery/internal/format"
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

	switch req.Variant {
	case variant.Raw:
		// No conversion: return original data with detected MIME type.
		mime := bimg.DetermineImageTypeName(req.Data)
		if mime == "" {
			mime = "application/octet-stream"
		}
		return &Result{Data: req.Data, ContentType: "image/" + mime}, nil

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

	default:
		return nil, fmt.Errorf("converter: unknown variant %v", req.Variant)
	}
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
