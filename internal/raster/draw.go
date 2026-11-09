package raster

import (
	"image"
	"image/color"
	"image/draw"
	"unicode/utf8"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Layout constants, matching the SVG renderer.
const (
	RowHeight  = 25 // pixels per row
	ColWidth   = 12 // pixels per column
	Padding    = 20 // padding around content
	HeaderSize = 2  // multiplier for header area (window buttons)
)

const (
	// baselineOffset is the distance from the bottom of a row to the text baseline.
	baselineOffset = 5

	// underlineOffset is the distance from the bottom of a row to the underline.
	underlineOffset = 2

	// windowButtonSpacing is the horizontal spacing between window buttons.
	windowButtonSpacing = 20

	// windowButtonRadius is the radius of window control buttons (close, minimize, maximize).
	windowButtonRadius = 6
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
		textWidth: utf8.RuneCountInString(run.Text) * ColWidth,
		x:         Padding + run.StartCol*ColWidth,
		y:         contentY + rowY*RowHeight,
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

	style := textRunStyleOf(run, rowY, catalog, r.contentOffsetY())

	if !catalog.IsDefault(run.Attrs.BG) {
		fillRect(img, image.Rect(style.x, style.y, style.x+style.textWidth, style.y+RowHeight), style.bg)
	}

	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{style.fg},
		Face: face,
		Dot:  fixed.P(style.x, style.y+RowHeight-baselineOffset),
	}
	drawer.DrawString(run.Text)

	if run.Attrs.Underline {
		underlineY := style.y + RowHeight - underlineOffset
		for px := style.x; px < style.x+style.textWidth; px++ {
			img.Set(px, underlineY, style.fg)
		}
	}
}

// drawCursor draws the cursor as a block in the default foreground color.
func (r *Rasterizer) drawCursor(img draw.Image, cursor ir.Cursor, catalog *termcolor.Catalog) {
	x := Padding + cursor.Col*ColWidth
	y := r.contentOffsetY() + cursor.Row*RowHeight

	fillRect(img, image.Rect(x, y, x+ColWidth, y+RowHeight), catalog.DefaultForeground())
}

func fillRect(img draw.Image, rect image.Rectangle, c color.RGBA) {
	draw.Draw(img, rect, &image.Uniform{c}, image.Point{}, draw.Src)
}

// drawWindow draws the window chrome: the background and the three buttons.
func (r *Rasterizer) drawWindow(img draw.Image) {
	theme := r.config.Theme

	fillRect(img, img.Bounds(), theme.WindowBackground)

	for i, btnColor := range theme.WindowButtons {
		drawCircle(img, Padding+i*windowButtonSpacing, Padding, windowButtonRadius, btnColor)
	}
}

// drawBackground draws a plain background without window chrome.
func (r *Rasterizer) drawBackground(img draw.Image) {
	fillRect(img, img.Bounds(), r.config.Theme.Background)
}

// drawTerminalBackground fills the terminal area with the theme's
// WindowBackground, so it is opaque and matches the window chrome.
func (r *Rasterizer) drawTerminalBackground(img draw.Image, width, height int) {
	y := r.contentOffsetY()
	fillRect(img, image.Rect(Padding, y, Padding+width, y+height), r.config.Theme.WindowBackground)
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

// contentOffsetY is the Y offset of the terminal content area.
func (r *Rasterizer) contentOffsetY() int {
	if r.config.ShowWindow {
		return Padding * HeaderSize
	}
	return Padding
}

func contentWidth(cols int) int { return cols * ColWidth }

func contentHeight(rows int) int { return rows * RowHeight }

// paddedWidth is the total image width including padding.
func paddedWidth(cols int) int { return contentWidth(cols) + 2*Padding }

// paddedHeight is the total image height including padding.
func (r *Rasterizer) paddedHeight(rows int) int {
	if r.config.ShowWindow {
		return contentHeight(rows) + Padding*HeaderSize + Padding
	}
	return contentHeight(rows) + 2*Padding
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
