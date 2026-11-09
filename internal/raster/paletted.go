package raster

import (
	"image"
	"image/color"
	"image/draw"
	"runtime"
	"sync"
	"time"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/layout"
	"github.com/mrmarble/termsvg/internal/progress"
	"golang.org/x/image/font"
)

// palettedFrameRenderer handles the parallel rendering of frames to paletted images.
type palettedFrameRenderer struct {
	rasterizer *Rasterizer
	rec        *ir.Recording
	palette    color.Palette
}

// render performs parallel frame rendering using a worker pool.
// Note: IR-level deduplication is handled during IR generation, not here.
//
//nolint:dupl // render methods for RGBA and Paletted are similar but use different image types
func (fr *palettedFrameRenderer) render() ([]PalettedFrame, error) {
	frames := fr.rec.Frames
	results := make([]PalettedFrame, len(frames))
	totalFrames := len(frames)

	contentWidth := fr.rec.Width * layout.ColWidth
	contentHeight := fr.rec.Height * layout.RowHeight
	width := layout.PaddedWidth(contentWidth)
	height := layout.PaddedHeight(contentHeight, fr.rasterizer.config.ShowWindow)

	// Pre-render the static base image (window chrome + terminal background) as paletted
	baseImg := fr.createPalettedBaseImage(width, height, contentWidth, contentHeight)

	progress.Send(fr.rasterizer.config.ProgressCh, "Rasterizing", 0, totalFrames)

	// Worker pool setup
	numWorkers := runtime.NumCPU()
	jobs := make(chan int, totalFrames)
	var wg sync.WaitGroup

	// Start workers - each worker creates its own font face
	for range numWorkers {
		wg.Go(func() {
			// Create own font face for this worker
			face, err := loadFontFace(float64(fr.rasterizer.config.FontSize), fr.rasterizer.sysFonts)
			if err != nil {
				// If font loading fails, use the shared one as fallback
				face = fr.rasterizer.fontFace
			}

			for idx := range jobs {
				results[idx] = fr.renderSingleFrame(idx, frames[idx], displayDelay(frames, idx), baseImg, face)

				progress.Send(fr.rasterizer.config.ProgressCh, "Rasterizing", idx+1, totalFrames)
			}
		})
	}

	for i := range frames {
		jobs <- i
	}
	close(jobs)

	wg.Wait()

	return results, nil
}

// renderSingleFrame renders a single frame to a paletted image.
func (fr *palettedFrameRenderer) renderSingleFrame(
	idx int,
	frame ir.Frame,
	delay time.Duration,
	baseImg *image.Paletted,
	face font.Face,
) PalettedFrame {
	img := fr.copyPalettedBaseImage(baseImg)

	// Draw the frame content directly to paletted using the worker's font face
	fr.drawFrameContentToPaletted(img, frame, face)

	return PalettedFrame{
		Image: img,
		Delay: delay,
		Index: idx,
	}
}

// createPalettedBaseImage creates the static base image with window chrome and terminal background.
// Uses WindowBackground color for the terminal area to match window chrome,
// ensuring full opacity for optimal GIF delta encoding performance.
func (fr *palettedFrameRenderer) createPalettedBaseImage(
	width, height, contentWidth, contentHeight int,
) *image.Paletted {
	// First create RGBA base image
	rgbaImg := image.NewRGBA(image.Rect(0, 0, width, height))

	// Draw window chrome or plain background
	if fr.rasterizer.config.ShowWindow {
		fr.rasterizer.drawWindow(rgbaImg)
	} else {
		fr.rasterizer.drawBackground(rgbaImg)
	}

	// Draw terminal content background using WindowBackground color
	// This ensures full opacity for GIF delta encoding while maintaining
	// visual consistency with the window chrome
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

	// Draw cursor if visible and cursor rendering is enabled
	if fr.rasterizer.config.ShowCursor && frame.Cursor.Visible {
		fr.rasterizer.drawCursorToPaletted(img, frame.Cursor, fr.rec.Colors)
	}
}
