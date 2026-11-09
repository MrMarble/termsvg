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

const (
	// baselineOffset is the distance from the bottom of a row to the text baseline.
	baselineOffset = 5

	// underlineOffset is the distance from the bottom of a row to the underline.
	underlineOffset = 2
)

// textRunStyle is where a text run goes and the colors it is drawn with.
type textRunStyle struct {
	fg, bg    color.RGBA
	textWidth int
	x, y      int
}

func textRunStyleOf(run ir.TextRun, rowY int, catalog *termcolor.Catalog, contentY int) textRunStyle {
	bgColor := catalog.DefaultBackground()
	if !catalog.IsDefault(run.Attrs.BG) {
		bgColor = catalog.Resolved(run.Attrs.BG)
	}

	fgColor := catalog.DefaultForeground()
	if !catalog.IsDefault(run.Attrs.FG) {
		fgColor = catalog.Resolved(run.Attrs.FG)
	}
	if run.Attrs.Dim {
		fgColor = dimColor(fgColor)
	}

	return textRunStyle{
		fg:        fgColor,
		bg:        bgColor,
		textWidth: utf8.RuneCountInString(run.Text) * layout.ColWidth,
		x:         layout.Padding + run.StartCol*layout.ColWidth,
		y:         contentY + rowY*layout.RowHeight,
	}
}

// drawTextRun draws a text run with the given font face. Each goroutine needs
// its own face. The same code serves RGBA and paletted images.
func (r *Rasterizer) drawTextRun(
	img draw.Image, run ir.TextRun, rowY int, face font.Face, catalog *termcolor.Catalog,
) {
	if run.Text == "" {
		return
	}

	style := textRunStyleOf(run, rowY, catalog, layout.ContentOffsetY(r.config.ShowWindow))

	if !catalog.IsDefault(run.Attrs.BG) {
		fillRect(img, image.Rect(style.x, style.y, style.x+style.textWidth, style.y+layout.RowHeight), style.bg)
	}

	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{style.fg},
		Face: face,
		Dot:  fixed.P(style.x, style.y+layout.RowHeight-baselineOffset),
	}
	drawer.DrawString(run.Text)

	if run.Attrs.Underline {
		underlineY := style.y + layout.RowHeight - underlineOffset
		for px := style.x; px < style.x+style.textWidth; px++ {
			img.Set(px, underlineY, style.fg)
		}
	}
}

// drawCursor draws the cursor as a block in the default foreground color.
func (r *Rasterizer) drawCursor(img draw.Image, cursor ir.Cursor, catalog *termcolor.Catalog) {
	x := layout.Padding + cursor.Col*layout.ColWidth
	y := layout.ContentOffsetY(r.config.ShowWindow) + cursor.Row*layout.RowHeight

	fillRect(img, image.Rect(x, y, x+layout.ColWidth, y+layout.RowHeight), catalog.DefaultForeground())
}

func fillRect(img draw.Image, rect image.Rectangle, c color.RGBA) {
	draw.Draw(img, rect, &image.Uniform{c}, image.Point{}, draw.Src)
}

// drawWindow draws the window chrome: the background and the three buttons.
func (r *Rasterizer) drawWindow(img draw.Image) {
	theme := r.config.Theme

	fillRect(img, img.Bounds(), theme.WindowBackground)

	for i, btnColor := range theme.WindowButtons {
		drawCircle(img, layout.Padding+i*layout.ButtonSpacing, layout.Padding, layout.ButtonRadius, btnColor)
	}
}

// drawBackground draws a plain background without window chrome.
func (r *Rasterizer) drawBackground(img draw.Image) {
	fillRect(img, img.Bounds(), r.config.Theme.Background)
}

// drawTerminalBackground fills the terminal area with the theme's
// WindowBackground, so it is opaque and matches the window chrome.
func (r *Rasterizer) drawTerminalBackground(img draw.Image, width, height int) {
	y := layout.ContentOffsetY(r.config.ShowWindow)
	fillRect(img, image.Rect(layout.Padding, y, layout.Padding+width, y+height), r.config.Theme.WindowBackground)
}

func drawCircle(img draw.Image, cx, cy, radius int, c color.RGBA) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= radius*radius {
				img.Set(cx+x, cy+y, c)
			}
		}
	}
}

// dimColor halves the RGB values. Unlike alpha blending it does not mix in the background.
func dimColor(c color.RGBA) color.RGBA {
	return color.RGBA{
		R: c.R / 2,
		G: c.G / 2,
		B: c.B / 2,
		A: c.A,
	}
}
