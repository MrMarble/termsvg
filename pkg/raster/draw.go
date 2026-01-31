package raster

import (
	"image"
	"image/color"
	"image/draw"
	"unicode/utf8"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// textRunColors extracts the foreground and background colors for a text run.
type textRunColors struct {
	fg, bg    color.RGBA
	textWidth int
	x, y      int
}

// Rendering constants for text positioning and styling.
const (
	// baselineOffset is the distance from the bottom of a row to the text baseline.
	// Text is drawn above the baseline, so we subtract this from row bottom.
	baselineOffset = 5

	// underlineOffset is the distance from the bottom of a row to the underline.
	underlineOffset = 2

	// windowCornerRadius is the radius for rounded window corners.
	windowCornerRadius = 5

	// windowButtonSpacing is the horizontal spacing between window buttons.
	windowButtonSpacing = 20

	// windowButtonRadius is the radius of window control buttons (close, minimize, maximize).
	windowButtonRadius = 6
)

// computeTextRunColors calculates the positioning and colors for a text run.
func (r *Rasterizer) computeTextRunColors(run ir.TextRun, rowY int, catalog *termcolor.Catalog) textRunColors {
	contentX := r.config.Padding
	contentY := r.contentOffsetY()

	x := contentX + run.StartCol*r.config.ColWidth
	y := contentY + rowY*r.config.RowHeight

	// Get background color - use catalog default background for unset cells
	var bgColor color.RGBA
	if catalog.IsDefault(run.Attrs.BG) {
		bgColor = catalog.DefaultBackground()
	} else {
		bgColor = catalog.Resolved(run.Attrs.BG)
	}

	// Get foreground color - use terminal foreground for default
	var fgColor color.RGBA
	if catalog.IsDefault(run.Attrs.FG) {
		fgColor = catalog.DefaultForeground()
	} else {
		fgColor = catalog.Resolved(run.Attrs.FG)
	}

	// Apply dim effect by reducing color intensity
	if run.Attrs.Dim {
		fgColor = dimColor(fgColor)
	}

	// Calculate text width in columns (handle multi-byte characters)
	textWidth := utf8.RuneCountInString(run.Text) * r.config.ColWidth

	return textRunColors{
		fg:        fgColor,
		bg:        bgColor,
		textWidth: textWidth,
		x:         x,
		y:         y,
	}
}

// drawTextRunWithFace draws a text run using the specified font face.
// This allows for thread-safe parallel rendering with per-goroutine font faces.
//
//nolint:dupl // drawTextRunWithFace and drawTextRunToPaletted handle different image types
func (r *Rasterizer) drawTextRunWithFace(
	img *image.RGBA, run ir.TextRun, rowY int, face font.Face, catalog *termcolor.Catalog,
) {
	if run.Text == "" {
		return
	}

	colors := r.computeTextRunColors(run, rowY, catalog)

	// Draw background rectangle for the run
	draw.Draw(img,
		image.Rect(colors.x, colors.y, colors.x+colors.textWidth, colors.y+r.config.RowHeight),
		&image.Uniform{colors.bg},
		image.Point{},
		draw.Src)

	// Draw text
	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{colors.fg},
		Face: face,
		Dot:  fixed.P(colors.x, colors.y+r.config.RowHeight-baselineOffset), // baseline offset
	}
	drawer.DrawString(run.Text)

	// Draw underline if needed
	if run.Attrs.Underline {
		underlineY := colors.y + r.config.RowHeight - underlineOffset
		for px := colors.x; px < colors.x+colors.textWidth; px++ {
			img.Set(px, underlineY, colors.fg)
		}
	}
}

// drawCursor draws the cursor as a filled block.
func (r *Rasterizer) drawCursor(img *image.RGBA, cursor ir.Cursor, catalog *termcolor.Catalog) {
	contentX := r.config.Padding
	contentY := r.contentOffsetY()

	x := contentX + cursor.Col*r.config.ColWidth
	y := contentY + cursor.Row*r.config.RowHeight

	// Get cursor color (same as default foreground)
	cursorColor := catalog.DefaultForeground()

	// Draw cursor as a block
	draw.Draw(img,
		image.Rect(x, y, x+r.config.ColWidth, y+r.config.RowHeight),
		&image.Uniform{cursorColor},
		image.Point{},
		draw.Src)
}

// drawWindow draws the window chrome including background and buttons.
func (r *Rasterizer) drawWindow(img *image.RGBA) {
	theme := r.config.Theme
	bounds := img.Bounds()

	// Window background with rounded corners
	r.drawRoundedRect(img, bounds, windowCornerRadius, theme.WindowBackground)

	// Window buttons (close, minimize, maximize)
	buttonY := r.config.Padding

	for i, btnColor := range theme.WindowButtons {
		x := r.config.Padding + i*windowButtonSpacing
		r.drawCircle(img, x, buttonY, windowButtonRadius, btnColor)
	}
}

// drawBackground draws a plain background without window chrome.
func (r *Rasterizer) drawBackground(img *image.RGBA) {
	bgColor := r.config.Theme.Background
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
}

// drawTerminalBackground draws the terminal content area background.
// Uses the theme's Background color for the terminal content.
func (r *Rasterizer) drawTerminalBackground(img *image.RGBA, width, height int) {
	contentX := r.config.Padding
	contentY := r.contentOffsetY()
	// Use theme Background for terminal content area
	termBg := r.config.Theme.Background

	draw.Draw(img,
		image.Rect(contentX, contentY, contentX+width, contentY+height),
		&image.Uniform{termBg},
		image.Point{},
		draw.Src)
}

// drawRoundedRect draws a rounded rectangle on the image.
// For simplicity, this draws a regular rectangle (visual difference is minimal at small radii).
func (r *Rasterizer) drawRoundedRect(img *image.RGBA, bounds image.Rectangle, radius int, c color.RGBA) {
	// Fill the main rectangle
	draw.Draw(img, bounds, &image.Uniform{c}, image.Point{}, draw.Src)

	// Note: A full implementation would use proper corner rounding algorithms.
	// The visual difference is minimal at small radii, so we use a simple rectangle.
	_ = radius // reserved for future implementation
}

// drawCircle draws a filled circle on the image.
func (r *Rasterizer) drawCircle(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= radius*radius {
				img.Set(cx+x, cy+y, c)
			}
		}
	}
}

// contentOffsetY returns the Y offset for the terminal content area.
func (r *Rasterizer) contentOffsetY() int {
	if r.config.ShowWindow {
		return r.config.Padding * r.config.HeaderSize
	}
	return r.config.Padding
}

// contentWidth calculates the width of the terminal content area.
func (r *Rasterizer) contentWidth(cols int) int {
	return cols * r.config.ColWidth
}

// contentHeight calculates the height of the terminal content area.
func (r *Rasterizer) contentHeight(rows int) int {
	return rows * r.config.RowHeight
}

// paddedWidth calculates the total image width including padding.
func (r *Rasterizer) paddedWidth(cols int) int {
	return r.contentWidth(cols) + 2*r.config.Padding
}

// paddedHeight calculates the total image height including padding.
func (r *Rasterizer) paddedHeight(rows int) int {
	if r.config.ShowWindow {
		return r.contentHeight(rows) + r.config.Padding*r.config.HeaderSize + r.config.Padding
	}
	return r.contentHeight(rows) + 2*r.config.Padding
}

// dimColor reduces the intensity of a color for the dim effect.
// Unlike alpha blending, this modifies the RGB values directly.
func dimColor(c color.RGBA) color.RGBA {
	return color.RGBA{
		R: c.R / 2,
		G: c.G / 2,
		B: c.B / 2,
		A: c.A,
	}
}

// drawTextRunToPaletted draws a text run directly to a paletted image.
// This avoids the RGBA to Paletted conversion step for GIF rendering.
//
//nolint:dupl // drawTextRunToPaletted is similar to drawTextRunWithFace but uses Paletted images
func (r *Rasterizer) drawTextRunToPaletted(
	img *image.Paletted, run ir.TextRun, rowY int, face font.Face, catalog *termcolor.Catalog,
) {
	if run.Text == "" {
		return
	}

	colors := r.computeTextRunColors(run, rowY, catalog)

	// Draw background rectangle for the run
	draw.Draw(img,
		image.Rect(colors.x, colors.y, colors.x+colors.textWidth, colors.y+r.config.RowHeight),
		&image.Uniform{colors.bg},
		image.Point{},
		draw.Src)

	// Draw text directly to paletted image
	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{colors.fg},
		Face: face,
		Dot:  fixed.P(colors.x, colors.y+r.config.RowHeight-baselineOffset), // baseline offset
	}
	drawer.DrawString(run.Text)

	// Draw underline if needed
	if run.Attrs.Underline {
		underlineY := colors.y + r.config.RowHeight - underlineOffset
		for px := colors.x; px < colors.x+colors.textWidth; px++ {
			img.Set(px, underlineY, colors.fg)
		}
	}
}

// drawCursorToPaletted draws the cursor as a filled block directly to a paletted image.
func (r *Rasterizer) drawCursorToPaletted(img *image.Paletted, cursor ir.Cursor, catalog *termcolor.Catalog) {
	contentX := r.config.Padding
	contentY := r.contentOffsetY()

	x := contentX + cursor.Col*r.config.ColWidth
	y := contentY + cursor.Row*r.config.RowHeight

	// Get cursor color (same as default foreground)
	cursorColor := catalog.DefaultForeground()

	// Draw cursor as a block
	draw.Draw(img,
		image.Rect(x, y, x+r.config.ColWidth, y+r.config.RowHeight),
		&image.Uniform{cursorColor},
		image.Point{},
		draw.Src)
}
