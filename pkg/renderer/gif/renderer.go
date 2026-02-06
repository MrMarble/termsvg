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
	"github.com/mrmarble/termsvg/pkg/progress"
	"github.com/mrmarble/termsvg/pkg/raster"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// Renderer implements the renderer.Renderer interface for GIF output.
type Renderer struct {
	config     renderer.Config
	rasterizer *raster.Rasterizer
}

// gifTimings holds timing measurements for GIF encoding.
type gifTimings struct {
	framesEqualTime   time.Duration
	computeDeltaTime  time.Duration
	framesEqualCalls  int
	computeDeltaCalls int
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
func New(config *renderer.Config) (*Renderer, error) {
	rasterizer, err := renderer.NewRasterizer(config)
	if err != nil {
		return nil, err
	}

	return &Renderer{
		config:     *config,
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

func (r *Renderer) sendProgress(current, total int) {
	if r.config.ProgressCh != nil {
		r.config.ProgressCh <- progress.Update{
			Phase:   "Encoding",
			Current: current,
			Total:   total,
		}
	}
}

// assembleGIF creates the final GIF from rendered paletted frames using delta encoding.
// GIF assembly requires multiple sequential steps for delta encoding.
//

func (r *Renderer) assembleGIF(frames []raster.PalettedFrame, w io.Writer) error {
	g := &gif.GIF{
		LoopCount: r.config.LoopCount,
	}

	var prevPaletted *image.Paletted
	totalFrames := len(frames)

	r.sendProgress(0, totalFrames)

	timings := &gifTimings{}

	for i, rf := range frames {
		delay := r.calculateDelay(rf.Delay, i, len(frames))

		if r.processFrame(g, prevPaletted, rf.Image, delay, timings) {
			continue
		}

		prevPaletted = rf.Image
		r.sendProgress(i+1, totalFrames)
	}

	return r.encodeAndLog(g, w, timings)
}

func (r *Renderer) calculateDelay(delay time.Duration, frameIdx, totalFrames int) int {
	d := int(delay.Milliseconds() / gifTimeUnit)
	if d < minGifDelay && frameIdx < totalFrames-1 {
		return minGifDelay
	}
	return d
}

func (r *Renderer) processFrame(g *gif.GIF, prev, curr *image.Paletted, delay int, t *gifTimings) bool {
	if prev != nil {
		feStart := time.Now()
		isEqual := framesEqual(prev, curr)
		if r.config.Debug {
			t.framesEqualTime += time.Since(feStart)
			t.framesEqualCalls++
		}
		if isEqual {
			g.Delay[len(g.Delay)-1] += delay
			return true
		}

		cdStart := time.Now()
		delta := computeDelta(prev, curr, 0)
		if r.config.Debug {
			t.computeDeltaTime += time.Since(cdStart)
			t.computeDeltaCalls++
		}
		g.Image = append(g.Image, delta)
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	} else {
		g.Image = append(g.Image, curr)
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}

	g.Delay = append(g.Delay, delay)
	return false
}

func (r *Renderer) encodeAndLog(g *gif.GIF, w io.Writer, t *gifTimings) error {
	encodeStart := time.Now()
	err := gif.EncodeAll(w, g)
	encodeTime := time.Since(encodeStart)

	if r.config.Debug {
		otherTime := time.Since(time.Now().Add(-encodeTime)) - t.framesEqualTime - t.computeDeltaTime - encodeTime
		log.Printf("[GIF] Phase 3 - GIF assembly breakdown:")
		log.Printf("[GIF]   - framesEqual: %v (%d calls)", t.framesEqualTime, t.framesEqualCalls)
		log.Printf("[GIF]   - computeDelta: %v (%d calls)", t.computeDeltaTime, t.computeDeltaCalls)
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
