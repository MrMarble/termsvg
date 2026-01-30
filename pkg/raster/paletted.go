package raster

import (
	"image"
	"image/color"
	"image/draw"
	"runtime"
	"sync"

	"golang.org/x/image/font"

	"github.com/mrmarble/termsvg/pkg/ir"
)

// palettedFrameRenderer handles the parallel rendering of frames to paletted images.
type palettedFrameRenderer struct {
	rasterizer *Rasterizer
	rec        *ir.Recording
	palette    color.Palette
}

// render performs parallel frame rendering with IR-level deduplication.
func (fr *palettedFrameRenderer) render() ([]PalettedFrame, error) {
	frames := fr.rec.Frames
	results := make([]PalettedFrame, len(frames))

	// Calculate image dimensions
	width := fr.rasterizer.paddedWidth(fr.rec.Width)
	height := fr.rasterizer.paddedHeight(fr.rec.Height)
	contentWidth := fr.rasterizer.contentWidth(fr.rec.Width)
	contentHeight := fr.rasterizer.contentHeight(fr.rec.Height)

	// Pre-render the static base image (window chrome + terminal background) as paletted
	baseImg := fr.createPalettedBaseImage(width, height, contentWidth, contentHeight)

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
			results[i] = PalettedFrame{
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
				results[idx] = PalettedFrame{
					Image:       nil,
					Delay:       delay,
					Index:       idx,
					IsDuplicate: true,
				}
				return
			}

			// Create a copy of the base paletted image for this frame
			img := fr.copyPalettedBaseImage(baseImg)

			// Draw the frame content directly to paletted
			fr.drawFrameContentToPaletted(img, frame, face)

			results[idx] = PalettedFrame{
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

// createPalettedBaseImage creates the static base image with window chrome and terminal background.
func (fr *palettedFrameRenderer) createPalettedBaseImage(width, height, contentWidth, contentHeight int) *image.Paletted {
	// First create RGBA base image
	rgbaImg := image.NewRGBA(image.Rect(0, 0, width, height))

	// Draw window chrome or plain background
	if fr.rasterizer.config.ShowWindow {
		fr.rasterizer.drawWindow(rgbaImg)
	} else {
		fr.rasterizer.drawBackground(rgbaImg)
	}

	// Draw terminal content background
	fr.rasterizer.drawTerminalBackground(rgbaImg, contentWidth, contentHeight)

	// Convert to paletted once (this is done only once per recording, not per frame)
	palettedImg := image.NewPaletted(rgbaImg.Bounds(), fr.palette)
	draw.Draw(palettedImg, rgbaImg.Bounds(), rgbaImg, image.Point{}, draw.Src)

	return palettedImg
}

// copyPalettedBaseImage creates a deep copy of the base paletted image.
func (fr *palettedFrameRenderer) copyPalettedBaseImage(base *image.Paletted) *image.Paletted {
	bounds := base.Bounds()
	img := image.NewPaletted(bounds, fr.palette)
	copy(img.Pix, base.Pix)
	return img
}

// drawFrameContentToPaletted draws the dynamic content (text runs and cursor) to a paletted image.
func (fr *palettedFrameRenderer) drawFrameContentToPaletted(img *image.Paletted, frame ir.Frame, face font.Face) {
	// Draw all text runs
	for _, row := range frame.Rows {
		for _, run := range row.Runs {
			fr.rasterizer.drawTextRunToPaletted(img, run, row.Y, face, fr.rec.Colors)
		}
	}

	// Draw cursor if visible
	if frame.Cursor.Visible {
		fr.rasterizer.drawCursorToPaletted(img, frame.Cursor, fr.rec.Colors)
	}
}

// computeRenderMask determines which frames need actual rendering.
// It performs IR-level deduplication by comparing frame content.
func (fr *palettedFrameRenderer) computeRenderMask(frames []ir.Frame) []bool {
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
