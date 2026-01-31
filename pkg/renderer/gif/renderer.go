// Package gif provides a GIF renderer for terminal recordings.
// It generates animated GIFs by rasterizing the terminal state frame by frame.
package gif

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"log"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/raster"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// Renderer implements the renderer.Renderer interface for GIF output.
type Renderer struct {
	config     renderer.Config
	rasterizer *raster.Rasterizer
}

// GIF timing constants.
const (
	// gifTimeUnit is the GIF delay time unit in milliseconds (10ms per unit).
	gifTimeUnit = 10

	// minGifDelay is the minimum delay value to avoid browser clamping.
	// Browsers clamp delays < 20ms to 100ms, so we use 2 units = 20ms.
	minGifDelay = 2
)

// New creates a new GIF renderer with the given configuration.
func New(config renderer.Config) (*Renderer, error) {
	rasterizer, err := renderer.NewRasterizer(config)
	if err != nil {
		return nil, err
	}

	return &Renderer{
		config:     config,
		rasterizer: rasterizer,
	}, nil
}

// Format returns the output format name.
func (r *Renderer) Format() string {
	return "gif"
}

// FileExtension returns the file extension for GIF files.
func (r *Renderer) FileExtension() string {
	return ".gif"
}

// Render generates an animated GIF from the recording.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	if len(rec.Frames) == 0 {
		return fmt.Errorf("recording has no frames")
	}

	startTime := time.Now()
	if r.config.Debug {
		log.Printf("[GIF] Starting GIF generation for %d frames", len(rec.Frames))
	}

	// Phase 1: Build the color palette for the GIF (needed before rendering)
	paletteStart := time.Now()
	palette := r.buildPalette(rec)
	if r.config.Debug {
		log.Printf("[GIF] Phase 1 - Palette building: %v (%d colors)", time.Since(paletteStart), len(palette))
	}

	// Phase 2: Use the raster package to render all frames directly to paletted images
	// This avoids the expensive RGBA -> Paletted conversion
	rasterStart := time.Now()
	palettedFrames, err := r.rasterizer.RasterizeWithPalette(rec, palette)
	if err != nil {
		return fmt.Errorf("failed to rasterize frames: %w", err)
	}
	rasterDuration := time.Since(rasterStart)

	if r.config.Debug {
		log.Printf("[GIF] Phase 2 - IR rasterization: %v (%d frames)",
			rasterDuration, len(palettedFrames))
	}

	// Check for cancellation after rendering
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Phase 3: Assemble the GIF from paletted frames
	assembleStart := time.Now()
	err = r.assembleGIF(palettedFrames, w)
	if err != nil {
		return err
	}
	if r.config.Debug {
		log.Printf("[GIF] Phase 3 - GIF assembly: %v", time.Since(assembleStart))
		log.Printf("[GIF] Total time: %v", time.Since(startTime))
	}

	return nil
}

// assembleGIF creates the final GIF from rendered paletted frames using delta encoding.
// GIF assembly requires multiple sequential steps for delta encoding.
func (r *Renderer) assembleGIF(frames []raster.PalettedFrame, w io.Writer) error {
	g := &gif.GIF{
		LoopCount: r.config.LoopCount,
	}

	var prevPaletted *image.Paletted

	// Timing accumulators for debug mode
	var framesEqualTime, computeDeltaTime time.Duration
	var framesEqualCalls, computeDeltaCalls int

	for i, rf := range frames {
		// Calculate delay for this frame (convert from time.Duration to GIF time units)
		delay := int(rf.Delay.Milliseconds() / gifTimeUnit)
		// Enforce minimum delay to avoid browser clamping
		if delay < minGifDelay && i < len(frames)-1 {
			delay = minGifDelay
		}

		// Pixel-level duplicate check (for frames that were rendered but are identical)
		if prevPaletted != nil {
			feStart := time.Now()
			isEqual := framesEqual(prevPaletted, rf.Image)
			if r.config.Debug {
				framesEqualTime += time.Since(feStart)
				framesEqualCalls++
			}
			if isEqual {
				g.Delay[len(g.Delay)-1] += delay
				continue
			}
		}

		// For delta encoding: if we have a previous frame, only encode changed pixels
		if prevPaletted != nil {
			cdStart := time.Now()
			delta := computeDelta(prevPaletted, rf.Image, 0) // 0 is transparent index
			if r.config.Debug {
				computeDeltaTime += time.Since(cdStart)
				computeDeltaCalls++
			}
			g.Image = append(g.Image, delta)
			g.Disposal = append(g.Disposal, gif.DisposalNone)
		} else {
			// First frame must be complete
			g.Image = append(g.Image, rf.Image)
			g.Disposal = append(g.Disposal, gif.DisposalNone)
		}

		g.Delay = append(g.Delay, delay)
		prevPaletted = rf.Image
	}

	encodeStart := time.Now()
	err := gif.EncodeAll(w, g)
	encodeTime := time.Since(encodeStart)

	// Log detailed timing breakdown in debug mode
	if r.config.Debug {
		otherTime := time.Since(time.Now().Add(-encodeTime)) - framesEqualTime - computeDeltaTime - encodeTime
		log.Printf("[GIF] Phase 3 - GIF assembly breakdown:")
		log.Printf("[GIF]   - framesEqual: %v (%d calls)", framesEqualTime, framesEqualCalls)
		log.Printf("[GIF]   - computeDelta: %v (%d calls)", computeDeltaTime, computeDeltaCalls)
		log.Printf("[GIF]   - gif.EncodeAll: %v", encodeTime)
		log.Printf("[GIF]   - other (loop overhead): %v", otherTime)
	}

	return err
}

// framesEqual checks if two paletted images are identical
func framesEqual(a, b *image.Paletted) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

// computeDelta creates a delta frame containing only pixels that changed
// Unchanged pixels are set to the transparent color index
func computeDelta(prev, curr *image.Paletted, transparentIdx uint8) *image.Paletted {
	bounds := curr.Bounds()
	delta := image.NewPaletted(bounds, curr.Palette)

	// Fill with transparent initially
	for i := range delta.Pix {
		delta.Pix[i] = transparentIdx
	}

	// Copy only changed pixels
	for i := range curr.Pix {
		if prev.Pix[i] != curr.Pix[i] {
			delta.Pix[i] = curr.Pix[i]
		}
	}

	return delta
}

// buildPalette creates a color palette from the recording's colors
func (r *Renderer) buildPalette(rec *ir.Recording) color.Palette {
	// Collect all unique colors
	colorSet := make(map[color.RGBA]bool)

	// Add theme colors
	colorSet[r.config.Theme.Background] = true
	colorSet[r.config.Theme.WindowBackground] = true
	colorSet[r.config.Theme.Foreground] = true
	for _, btnColor := range r.config.Theme.WindowButtons {
		colorSet[btnColor] = true
	}

	// Add colors from the color catalog
	colorSet[rec.Colors.DefaultForeground()] = true
	colorSet[rec.Colors.DefaultBackground()] = true
	for _, rgba := range rec.Colors.All() {
		colorSet[rgba] = true
	}

	// Convert to palette
	palette := make(color.Palette, 0, len(colorSet)+1)

	// Add transparent color first (for potential optimization)
	palette = append(palette, color.RGBA{0, 0, 0, 0})

	for c := range colorSet {
		palette = append(palette, c)
	}

	// If palette is too small, pad with black
	for len(palette) < 2 {
		palette = append(palette, color.RGBA{0, 0, 0, 255})
	}

	// GIF supports max 256 colors - if we have more, the quantizer will handle it
	if len(palette) > 256 {
		palette = palette[:256]
	}

	return palette
}
