package raster

import (
	"image"
	"runtime"
	"sync"

	"golang.org/x/image/font"

	"github.com/mrmarble/termsvg/pkg/ir"
)

// frameRenderer handles the parallel rendering of frames.
type frameRenderer struct {
	rasterizer *Rasterizer
	rec        *ir.Recording
}

// render performs parallel frame rendering with IR-level deduplication.
func (fr *frameRenderer) render() ([]RasterFrame, error) {
	frames := fr.rec.Frames
	results := make([]RasterFrame, len(frames))

	// Calculate image dimensions
	width := fr.rasterizer.paddedWidth(fr.rec.Width)
	height := fr.rasterizer.paddedHeight(fr.rec.Height)
	contentWidth := fr.rasterizer.contentWidth(fr.rec.Width)
	contentHeight := fr.rasterizer.contentHeight(fr.rec.Height)

	// Pre-render the static base image (window chrome + terminal background)
	baseImg := fr.createBaseImage(width, height, contentWidth, contentHeight)

	// Determine which frames need rendering (IR-level deduplication)
	needsRender := fr.computeRenderMask(frames)

	// Use worker pool to limit concurrency
	numWorkers := runtime.NumCPU()
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	for i := range frames {
		// Calculate delay for this frame
		delay := frames[i].Delay

		if !needsRender[i] {
			// IR-level duplicate: mark as duplicate, no image needed
			results[i] = RasterFrame{
				Image:       nil,
				Delay:       delay,
				Index:       i,
				IsDuplicate: true,
			}
			continue
		}

		wg.Add(1)
		go func(idx int, frame ir.Frame, frameDelay int64) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			// Create a per-goroutine font face (font.Face is not thread-safe)
			face, err := loadFontFace(float64(fr.rasterizer.config.FontSize))
			if err != nil {
				// In case of error, mark as duplicate to avoid crashing
				results[idx] = RasterFrame{
					Image:       nil,
					Delay:       delay,
					Index:       idx,
					IsDuplicate: true,
				}
				return
			}

			// Create a copy of the base image for this frame
			img := fr.copyBaseImage(baseImg)

			// Draw the frame content
			fr.drawFrameContent(img, frame, face)

			results[idx] = RasterFrame{
				Image:       img,
				Delay:       delay,
				Index:       idx,
				IsDuplicate: false,
			}
		}(i, frames[i], int64(delay))
	}

	wg.Wait()
	return results, nil
}

// createBaseImage creates the static base image with window chrome and terminal background.
func (fr *frameRenderer) createBaseImage(width, height, contentWidth, contentHeight int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Draw window chrome or plain background
	if fr.rasterizer.config.ShowWindow {
		fr.rasterizer.drawWindow(img)
	} else {
		fr.rasterizer.drawBackground(img)
	}

	// Draw terminal content background
	fr.rasterizer.drawTerminalBackground(img, contentWidth, contentHeight)

	return img
}

// copyBaseImage creates a deep copy of the base image.
func (fr *frameRenderer) copyBaseImage(base *image.RGBA) *image.RGBA {
	bounds := base.Bounds()
	img := image.NewRGBA(bounds)
	copy(img.Pix, base.Pix)
	return img
}

// drawFrameContent draws the dynamic content (text runs and cursor) to an image.
func (fr *frameRenderer) drawFrameContent(img *image.RGBA, frame ir.Frame, face font.Face) {
	// Draw all text runs
	for _, row := range frame.Rows {
		for _, run := range row.Runs {
			fr.rasterizer.drawTextRunWithFace(img, run, row.Y, face, fr.rec.Colors)
		}
	}

	// Draw cursor if visible
	if frame.Cursor.Visible {
		fr.rasterizer.drawCursor(img, frame.Cursor, fr.rec.Colors)
	}
}

// computeRenderMask determines which frames need actual rendering.
// It performs IR-level deduplication by comparing frame content.
func (fr *frameRenderer) computeRenderMask(frames []ir.Frame) []bool {
	needsRender := make([]bool, len(frames))
	needsRender[0] = true // First frame always needs rendering

	var prevFrame *ir.Frame
	for i := range frames {
		if i == 0 {
			prevFrame = &frames[0]
			continue
		}
		// IR-level comparison: skip rendering if frame content is identical
		if !framesEqualIR(prevFrame, &frames[i]) {
			needsRender[i] = true
			prevFrame = &frames[i]
		}
	}

	return needsRender
}

// framesEqualIR compares two IR frames for equality without rendering.
// This is much faster than pixel comparison since it operates on the IR data.
func framesEqualIR(a, b *ir.Frame) bool {
	// Compare cursor state
	if a.Cursor != b.Cursor {
		return false
	}

	// Compare row count
	if len(a.Rows) != len(b.Rows) {
		return false
	}

	// Compare each row
	for i := range a.Rows {
		if !rowsEqualIR(&a.Rows[i], &b.Rows[i]) {
			return false
		}
	}

	return true
}

// rowsEqualIR compares two IR rows for equality.
func rowsEqualIR(a, b *ir.Row) bool {
	if a.Y != b.Y {
		return false
	}

	if len(a.Runs) != len(b.Runs) {
		return false
	}

	for i := range a.Runs {
		if !textRunsEqualIR(&a.Runs[i], &b.Runs[i]) {
			return false
		}
	}

	return true
}

// textRunsEqualIR compares two IR text runs for equality.
func textRunsEqualIR(a, b *ir.TextRun) bool {
	return a.Text == b.Text &&
		a.StartCol == b.StartCol &&
		a.Attrs == b.Attrs
}
