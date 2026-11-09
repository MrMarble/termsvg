// Package raster transforms terminal recordings (IR) into RGBA images.
// It provides parallel frame rendering with IR-level deduplication,
// supporting both window chrome and plain terminal output.
package raster

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/renderer"
	"golang.org/x/image/font"
)

// RasterFrame is a rendered RGBA frame with its display time.
//
//nolint:revive // RasterFrame naming is intentional to distinguish from IR frames
type RasterFrame struct {
	Image *image.RGBA
	Delay time.Duration

	// Index is the frame number (0-indexed, sequential after deduplication)
	Index int
}

// Rasterizer transforms IR recordings into RGBA images.
type Rasterizer struct {
	config   renderer.Config
	fontFace font.Face
	sysFonts *systemFonts // nil unless config.SystemFonts
}

// PalettedFrame represents a single rendered frame as a paletted image with timing metadata.
type PalettedFrame struct {
	// Image is the rendered paletted image for this frame
	Image *image.Paletted

	// Delay is the time to display this frame
	Delay time.Duration

	// Index is the frame number (0-indexed, sequential after deduplication)
	Index int
}

// displayDelay is how long frame idx of frames stays on screen.
func displayDelay(frames []ir.Frame, idx int) time.Duration {
	delay := frames[idx].Delay
	if idx == len(frames)-1 {
		delay = max(delay, LastFrameHold)
	}

	return delay
}

// New creates a Rasterizer for the given renderer configuration.
func New(config *renderer.Config) (*Rasterizer, error) {
	var sys *systemFonts
	if config.SystemFonts {
		preferred, dirs := defaultFontDirs()
		sys = newSystemFonts(preferred, dirs)
	}

	face, err := loadFontFace(float64(config.FontSize), sys)
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %w", err)
	}

	return &Rasterizer{
		config:   *config,
		fontFace: face,
		sysFonts: sys,
	}, nil
}

// Close releases resources held by the rasterizer.
func (r *Rasterizer) Close() error {
	// font.Face doesn't have a Close method
	// Resource cleanup can be added here if needed in the future
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
