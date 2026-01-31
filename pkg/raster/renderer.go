package raster

import (
	"image"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"golang.org/x/image/font"
)

// frameRenderer handles the parallel rendering of frames.
type frameRenderer struct {
	rasterizer *Rasterizer
	rec        *ir.Recording
}

// render performs parallel frame rendering.
// Note: IR-level deduplication is handled during IR generation, not here.
//
//nolint:dupl // render methods for RGBA and Paletted are similar but use different types
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

	// Render all frames (IR is already deduplicated)
	for i := range frames {
		results[i] = fr.renderSingleFrame(i, frames[i], frames[i].Delay, baseImg)
	}

	return results, nil
}

// renderSingleFrame renders a single frame to an RGBA image.
func (fr *frameRenderer) renderSingleFrame(
	idx int,
	frame ir.Frame,
	delay time.Duration,
	baseImg *image.RGBA,
) RasterFrame {
	// Create a copy of the base image for this frame
	img := fr.copyBaseImage(baseImg)

	// Draw the frame content using the cached font face
	fr.drawFrameContent(img, frame, fr.rasterizer.fontFace)

	return RasterFrame{
		Image: img,
		Delay: delay,
		Index: idx,
	}
}

// createBaseImage creates the static base image with window chrome and terminal background.
// Uses WindowBackground color for the terminal area to match window chrome,
// ensuring full opacity for optimal GIF delta encoding performance.
func (fr *frameRenderer) createBaseImage(width, height, contentWidth, contentHeight int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Draw window chrome or plain background
	if fr.rasterizer.config.ShowWindow {
		fr.rasterizer.drawWindow(img)
	} else {
		fr.rasterizer.drawBackground(img)
	}

	// Draw terminal content background using WindowBackground color
	// This ensures full opacity for GIF delta encoding while maintaining
	// visual consistency with the window chrome
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

	// Draw cursor if visible and cursor rendering is enabled
	if fr.rasterizer.config.ShowCursor && frame.Cursor.Visible {
		fr.rasterizer.drawCursor(img, frame.Cursor, fr.rec.Colors)
	}
}
