package raster

import (
	"image"
	"image/color"
	"image/draw"
	"unicode/utf8"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/layout"
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
)

// computeTextRunColors calculates the positioning and colors for a text run.
func (r *Rasterizer) computeTextRunColors(run ir.TextRun, rowY int, catalog *termcolor.Catalog) textRunColors {
	contentX := layout.Padding
	contentY := layout.ContentOffsetY(r.config.ShowWindow)

	x := contentX + run.StartCol*layout.ColWidth
	y := contentY + rowY*layout.RowHeight

	var bgColor color.RGBA
	if catalog.IsDefault(run.Attrs.BG) {
		bgColor = catalog.DefaultBackground()
	} else {
		bgColor = catalog.Resolved(run.Attrs.BG)
	}

	var fgColor color.RGBA
	if catalog.IsDefault(run.Attrs.FG) {
		fgColor = catalog.DefaultForeground()
	} else {
		fgColor = catalog.Resolved(run.Attrs.FG)
	}

	if run.Attrs.Dim {
		fgColor = dimColor(fgColor)
	}

	textWidth := utf8.RuneCountInString(run.Text) * layout.ColWidth

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

	// The default background is already painted.
	if !catalog.IsDefault(run.Attrs.BG) {
		draw.Draw(img,
			image.Rect(colors.x, colors.y, colors.x+colors.textWidth, colors.y+layout.RowHeight),
			&image.Uniform{colors.bg},
			image.Point{},
			draw.Src)
	}

	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{colors.fg},
		Face: face,
		Dot:  fixed.P(colors.x, colors.y+layout.RowHeight-baselineOffset),
	}
	drawer.DrawString(run.Text)

	if run.Attrs.Underline {
		underlineY := colors.y + layout.RowHeight - underlineOffset
		for px := colors.x; px < colors.x+colors.textWidth; px++ {
			img.Set(px, underlineY, colors.fg)
		}
	}
}

// drawCursor draws the cursor as a block in the default foreground color.
func (r *Rasterizer) drawCursor(img *image.RGBA, cursor ir.Cursor, catalog *termcolor.Catalog) {
	contentX := layout.Padding
	contentY := layout.ContentOffsetY(r.config.ShowWindow)

	x := contentX + cursor.Col*layout.ColWidth
	y := contentY + cursor.Row*layout.RowHeight

	cursorColor := catalog.DefaultForeground()

	draw.Draw(img,
		image.Rect(x, y, x+layout.ColWidth, y+layout.RowHeight),
		&image.Uniform{cursorColor},
		image.Point{},
		draw.Src)
}

// drawWindow fills the image with the window background and draws the buttons.
// The corners stay square, unlike in the SVG.
func (r *Rasterizer) drawWindow(img *image.RGBA) {
	theme := r.config.Theme
	draw.Draw(img, img.Bounds(), &image.Uniform{theme.WindowBackground}, image.Point{}, draw.Src)

	for i, btnColor := range theme.WindowButtons {
		x := layout.Padding + i*layout.ButtonSpacing
		r.drawCircle(img, x, layout.Padding, layout.ButtonRadius, btnColor)
	}
}

// drawBackground draws a plain background without window chrome.
func (r *Rasterizer) drawBackground(img *image.RGBA) {
	bgColor := r.config.Theme.Background
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
}

// drawTerminalBackground draws the terminal content area background.
// Uses the theme's WindowBackground color to match the window chrome,
// creating a seamless appearance while maintaining full opacity for GIF performance.
func (r *Rasterizer) drawTerminalBackground(img *image.RGBA, width, height int) {
	contentX := layout.Padding
	contentY := layout.ContentOffsetY(r.config.ShowWindow)
	termBg := r.config.Theme.WindowBackground

	draw.Draw(img,
		image.Rect(contentX, contentY, contentX+width, contentY+height),
		&image.Uniform{termBg},
		image.Point{},
		draw.Src)
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

	// The default background is already painted.
	if !catalog.IsDefault(run.Attrs.BG) {
		draw.Draw(img,
			image.Rect(colors.x, colors.y, colors.x+colors.textWidth, colors.y+layout.RowHeight),
			&image.Uniform{colors.bg},
			image.Point{},
			draw.Src)
	}

	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{colors.fg},
		Face: face,
		Dot:  fixed.P(colors.x, colors.y+layout.RowHeight-baselineOffset),
	}
	drawer.DrawString(run.Text)

	if run.Attrs.Underline {
		underlineY := colors.y + layout.RowHeight - underlineOffset
		for px := colors.x; px < colors.x+colors.textWidth; px++ {
			img.Set(px, underlineY, colors.fg)
		}
	}
}

// drawCursorToPaletted draws the cursor as a filled block directly to a paletted image.
func (r *Rasterizer) drawCursorToPaletted(img *image.Paletted, cursor ir.Cursor, catalog *termcolor.Catalog) {
	contentX := layout.Padding
	contentY := layout.ContentOffsetY(r.config.ShowWindow)

	x := contentX + cursor.Col*layout.ColWidth
	y := contentY + cursor.Row*layout.RowHeight

	cursorColor := catalog.DefaultForeground()

	draw.Draw(img,
		image.Rect(x, y, x+layout.ColWidth, y+layout.RowHeight),
		&image.Uniform{cursorColor},
		image.Point{},
		draw.Src)
}
