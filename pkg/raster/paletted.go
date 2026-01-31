package raster

import (
	"image"
	"image/color"
	"image/draw"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"golang.org/x/image/font"
)

// palettedFrameRenderer handles the parallel rendering of frames to paletted images.
type palettedFrameRenderer struct {
	rasterizer *Rasterizer
	rec        *ir.Recording
	palette    color.Palette
}

// render performs parallel frame rendering with IR-level deduplication.
//
// render performs parallel frame rendering.
// Note: IR-level deduplication is handled during IR generation, not here.
//
//nolint:dupl // render methods for RGBA and Paletted are similar but use different image types
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

	// Render all frames (IR is already deduplicated)
	for i := range frames {
		results[i] = fr.renderSingleFrame(i, frames[i], frames[i].Delay, baseImg)
	}

	return results, nil
}

// renderSingleFrame renders a single frame to a paletted image.
func (fr *palettedFrameRenderer) renderSingleFrame(
	idx int,
	frame ir.Frame,
	delay time.Duration,
	baseImg *image.Paletted,
) PalettedFrame {
	// Create a per-goroutine font face (font.Face is not thread-safe)
	face, err := loadFontFace(float64(fr.rasterizer.config.FontSize))
	if err != nil {
		// In case of error, return empty frame
		return PalettedFrame{
			Image: nil,
			Delay: delay,
			Index: idx,
		}
	}

	// Create a copy of the base paletted image for this frame
	img := fr.copyPalettedBaseImage(baseImg)

	// Draw the frame content directly to paletted
	fr.drawFrameContentToPaletted(img, frame, face)

	return PalettedFrame{
		Image: img,
		Delay: delay,
		Index: idx,
	}
}

// createPalettedBaseImage creates the static base image with window chrome
// and terminal background.
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
