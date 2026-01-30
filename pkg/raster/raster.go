// Package raster transforms terminal recordings (IR) into RGBA images.
// It provides parallel frame rendering with IR-level deduplication,
// supporting both window chrome and plain terminal output.
package raster

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"golang.org/x/image/font"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/theme"
)

// Layout constants for rendering (matching SVG renderer for consistency)
const (
	RowHeight  = 25 // pixels per row
	ColWidth   = 12 // pixels per column
	Padding    = 20 // padding around content
	HeaderSize = 2  // multiplier for header area (window buttons)
)

// RasterFrame represents a single rendered frame with timing metadata.
type RasterFrame struct {
	// Image is the rendered RGBA image for this frame
	Image *image.RGBA

	// Delay is the time to display this frame
	Delay time.Duration

	// Index is the original frame number from the recording
	Index int

	// IsDuplicate indicates if this frame is identical to the previous frame
	// The Image field may be nil for duplicates (caller should use previous frame's image)
	IsDuplicate bool
}

// Config holds configuration options for the rasterizer.
type Config struct {
	// Theme defines the color scheme for rendering
	Theme theme.Theme

	// ShowWindow enables window chrome rendering (macOS-style buttons)
	ShowWindow bool

	// FontSize is the font size in points
	FontSize int

	// Layout constants (can be overridden, but defaults are provided)
	RowHeight  int // pixels per row (default: 25)
	ColWidth   int // pixels per column (default: 12)
	Padding    int // padding around content (default: 20)
	HeaderSize int // multiplier for header area (default: 2)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Theme:      theme.Default(),
		ShowWindow: true,
		FontSize:   20,
		RowHeight:  RowHeight,
		ColWidth:   ColWidth,
		Padding:    Padding,
		HeaderSize: HeaderSize,
	}
}

// Rasterizer transforms IR recordings into RGBA images.
type Rasterizer struct {
	config   Config
	fontFace font.Face
}

// New creates a new Rasterizer with the given configuration.
func New(config Config) (*Rasterizer, error) {
	face, err := loadFontFace(float64(config.FontSize))
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %w", err)
	}

	return &Rasterizer{
		config:   config,
		fontFace: face,
	}, nil
}

// Close releases resources held by the rasterizer.
func (r *Rasterizer) Close() error {
	if r.fontFace != nil {
		// font.Face doesn't have a Close method, but we could add
		// resource cleanup here if needed in the future
	}
	return nil
}

// Rasterize transforms a terminal recording into a series of RGBA images.
// It performs IR-level deduplication to avoid rendering identical frames.
// The returned slice contains all frames with their timing metadata.
func (r *Rasterizer) Rasterize(rec *ir.Recording) ([]RasterFrame, error) {
	if len(rec.Frames) == 0 {
		return nil, fmt.Errorf("recording has no frames")
	}

	renderer := &frameRenderer{
		rasterizer: r,
		rec:        rec,
	}

	return renderer.render()
}

// PalettedFrame represents a single rendered frame as a paletted image with timing metadata.
type PalettedFrame struct {
	// Image is the rendered paletted image for this frame
	Image *image.Paletted

	// Delay is the time to display this frame
	Delay time.Duration

	// Index is the original frame number from the recording
	Index int

	// IsDuplicate indicates if this frame is identical to the previous frame
	// The Image field may be nil for duplicates (caller should use previous frame's image)
	IsDuplicate bool
}

// RasterizeWithPalette transforms a terminal recording into a series of paletted images.
// It renders directly to paletted images using the provided palette, avoiding the
// expensive RGBA to Paletted conversion step. This is optimal for GIF generation.
// It performs IR-level deduplication to avoid rendering identical frames.
func (r *Rasterizer) RasterizeWithPalette(rec *ir.Recording, palette color.Palette) ([]PalettedFrame, error) {
	if len(rec.Frames) == 0 {
		return nil, fmt.Errorf("recording has no frames")
	}

	renderer := &palettedFrameRenderer{
		rasterizer: r,
		rec:        rec,
		palette:    palette,
	}

	return renderer.render()
}
