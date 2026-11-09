// Package raster transforms terminal recordings (IR) into RGBA or paletted
// images. It renders frames in parallel, with or without window chrome.
package raster

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"time"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/renderer"
)

// RasterFrame is a rendered RGBA frame and how long it is shown.
//
//nolint:revive // RasterFrame naming is intentional to distinguish from IR frames
type RasterFrame struct {
	Image *image.RGBA
	Delay time.Duration
}

// PalettedFrame is a rendered paletted frame and how long it is shown.
type PalettedFrame struct {
	Image *image.Paletted
	Delay time.Duration
}

// Rasterizer transforms IR recordings into images.
type Rasterizer struct {
	config   renderer.Config
	sysFonts *systemFonts // nil unless config.SystemFonts
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

	// Fail here, rather than in a worker, when the font cannot be loaded.
	face, err := loadFontFace(float64(config.FontSize), sys)
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %w", err)
	}
	_ = face.Close()

	return &Rasterizer{config: *config, sysFonts: sys}, nil
}

// Rasterize renders a recording as RGBA images, one per frame.
func (r *Rasterizer) Rasterize(rec *ir.Recording) ([]RasterFrame, error) {
	if len(rec.Frames) == 0 {
		return nil, fmt.Errorf("recording has no frames")
	}

	images, err := renderFrames(r, rec, r.baseImage(rec), func(base *image.RGBA) *image.RGBA {
		img := image.NewRGBA(base.Bounds())
		copy(img.Pix, base.Pix)
		return img
	})
	if err != nil {
		return nil, err
	}

	frames := make([]RasterFrame, len(images))
	for i, img := range images {
		frames[i] = RasterFrame{Image: img, Delay: displayDelay(rec.Frames, i)}
	}

	return frames, nil
}

// RasterizeWithPalette renders a recording as paletted images, one per frame,
// drawing straight into the palette rather than converting RGBA images. This is
// what makes GIF rendering fast.
func (r *Rasterizer) RasterizeWithPalette(rec *ir.Recording, palette color.Palette) ([]PalettedFrame, error) {
	if len(rec.Frames) == 0 {
		return nil, fmt.Errorf("recording has no frames")
	}

	rgba := r.baseImage(rec)
	base := image.NewPaletted(rgba.Bounds(), palette)
	draw.Draw(base, rgba.Bounds(), rgba, image.Point{}, draw.Src)

	images, err := renderFrames(r, rec, base, func(base *image.Paletted) *image.Paletted {
		img := image.NewPaletted(base.Bounds(), palette)
		copy(img.Pix, base.Pix)
		return img
	})
	if err != nil {
		return nil, err
	}

	frames := make([]PalettedFrame, len(images))
	for i, img := range images {
		frames[i] = PalettedFrame{Image: img, Delay: displayDelay(rec.Frames, i)}
	}

	return frames, nil
}
