// Package gif provides a GIF renderer for terminal recordings.
// It generates animated GIFs by rasterizing the terminal state frame by frame.
package gif

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"log"
	"time"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/raster"
	"github.com/mrmarble/termsvg/internal/renderer"
)

// Renderer implements the renderer.Renderer interface for GIF output.
type Renderer struct {
	config     renderer.Config
	rasterizer *raster.Rasterizer
}

const (
	// gifTimeUnit is the GIF delay time unit in milliseconds (10ms per unit).
	gifTimeUnit = 10

	// minGifDelay is the minimum delay value to avoid browser clamping.
	// Browsers clamp delays < 20ms to 100ms, so we use 2 units = 20ms.
	minGifDelay = 2

	// maxGIFColors is the size of a GIF palette.
	maxGIFColors = 256
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

// Render generates an animated GIF from the recording.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	if len(rec.Frames) == 0 {
		return fmt.Errorf("recording has no frames")
	}

	start := time.Now()
	// The palette has to be known before the frames are drawn, so they can be
	// drawn straight into it.
	frames, err := r.rasterizer.RasterizeWithPalette(rec, r.buildPalette(rec))
	if err != nil {
		return fmt.Errorf("failed to rasterize frames: %w", err)
	}
	rasterized := time.Since(start)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := r.assembleGIF(frames, w); err != nil {
		return err
	}
	if r.config.Debug {
		log.Printf("[GIF] %d frames: rasterized in %v, assembled in %v",
			len(frames), rasterized, time.Since(start)-rasterized)
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

// assembleGIF writes the frames as a GIF in which each frame after the first
// holds only the pixels that differ from the one before it.
func (r *Renderer) assembleGIF(frames []raster.PalettedFrame, w io.Writer) error {
	g := &gif.GIF{LoopCount: r.config.LoopCount}
	var prev *image.Paletted

	r.sendProgress(0, len(frames))

	for i, frame := range frames {
		delay := gifDelay(frame.Delay, i == len(frames)-1)

		switch {
		case prev == nil:
			g.Image = append(g.Image, frame.Image)
		case framesEqual(prev, frame.Image):
			g.Delay[len(g.Delay)-1] += delay
			continue
		default:
			g.Image = append(g.Image, changedPixels(prev, frame.Image))
		}
		g.Disposal = append(g.Disposal, gif.DisposalNone)
		g.Delay = append(g.Delay, delay)

		prev = frame.Image
		r.sendProgress(i+1, len(frames))
	}

	return gif.EncodeAll(w, g)
}

// gifDelay converts a display time to GIF delay units. Every frame but the
// last is shown for at least minGifDelay.
func gifDelay(delay time.Duration, last bool) int {
	units := int(delay.Milliseconds() / gifTimeUnit)
	if units < minGifDelay && !last {
		return minGifDelay
	}

	return units
}

func framesEqual(a, b *image.Paletted) bool {
	return a.Bounds() == b.Bounds() && bytes.Equal(a.Pix, b.Pix)
}

// changedPixels returns an image of curr's size and palette that has curr's
// pixels where they differ from prev, and the transparent color (index 0) elsewhere.
func changedPixels(prev, curr *image.Paletted) *image.Paletted {
	delta := image.NewPaletted(curr.Bounds(), curr.Palette)
	for i := range curr.Pix {
		if prev.Pix[i] != curr.Pix[i] {
			delta.Pix[i] = curr.Pix[i]
		}
	}

	return delta
}

// buildPalette creates the GIF colour palette: a transparent entry, then the
// theme colours, then the recording's colours in the order they first appear.
// The order is deterministic, and when there are more than 256 colours the
// ones dropped are always the most recently introduced.
func (r *Renderer) buildPalette(rec *ir.Recording) color.Palette {
	palette := make(color.Palette, 0, maxGIFColors)
	seen := make(map[color.RGBA]bool)
	palette = append(palette, color.RGBA{})
	add := func(c color.RGBA) {
		if !seen[c] && len(palette) < maxGIFColors {
			seen[c] = true
			palette = append(palette, c)
		}
	}

	theme := &r.config.Theme
	add(theme.Background)
	add(theme.WindowBackground)
	add(theme.Foreground)
	for _, btnColor := range theme.WindowButtons {
		add(btnColor)
	}

	add(rec.Colors.DefaultForeground())
	add(rec.Colors.DefaultBackground())
	for _, c := range rec.Colors.Colors() {
		add(c)
	}

	// A GIF needs at least two colours.
	for len(palette) < 2 {
		palette = append(palette, color.RGBA{A: 255})
	}

	return palette
}
