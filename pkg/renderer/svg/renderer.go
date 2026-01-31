// Package svg provides an SVG renderer for terminal recordings.
// It generates animated SVGs using CSS keyframes to translate between frames.
package svg

import (
	"context"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// Renderer implements the renderer.Renderer interface for SVG output.
type Renderer struct {
	config renderer.Config
}

// canvas holds rendering state
type canvas struct {
	w          io.Writer
	rec        *ir.Recording
	config     renderer.Config
	classNames map[color.ID]string
}

// Layout constants for SVG rendering
const (
	RowHeight  = 25 // pixels per row
	ColWidth   = 12 // pixels per column
	Padding    = 20 // padding around content
	HeaderSize = 2  // multiplier for header area (window buttons)

	// windowCornerRadius is the radius for rounded window corners.
	windowCornerRadius = 5

	// windowButtonSpacing is the horizontal spacing between window buttons.
	windowButtonSpacing = 20

	// windowButtonRadius is the radius of window control buttons.
	windowButtonRadius = 6
)

// New creates a new SVG renderer with the given configuration.
func New(config renderer.Config) *Renderer {
	return &Renderer{config: config}
}

// Format returns the output format name.
func (r *Renderer) Format() string {
	return "svg"
}

// FileExtension returns the file extension for SVG files.
func (r *Renderer) FileExtension() string {
	return ".svg"
}

// Render generates an animated SVG from the recording.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	if len(rec.Frames) == 0 {
		return fmt.Errorf("recording has no frames")
	}

	c := &canvas{
		w:          w,
		rec:        rec,
		config:     r.config,
		classNames: rec.Colors.GenerateClassNames(),
	}

	return c.render(ctx)
}

func (c *canvas) contentWidth() int {
	return c.rec.Width * ColWidth
}

func (c *canvas) contentHeight() int {
	return c.rec.Height * RowHeight
}

func (c *canvas) paddedWidth() int {
	return c.contentWidth() + 2*Padding
}

func (c *canvas) paddedHeight() int {
	if c.config.ShowWindow {
		return c.contentHeight() + Padding*HeaderSize + Padding
	}
	return c.contentHeight() + 2*Padding
}

func (c *canvas) render(ctx context.Context) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// SVG header
	width := c.paddedWidth()
	height := c.paddedHeight()
	fmt.Fprintf(c.w, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">`, width, height)

	if c.config.ShowWindow {
		c.writeWindow()
	} else {
		c.writeBackground()
	}

	// Content group with clipping
	contentY := Padding
	if c.config.ShowWindow {
		contentY = Padding * HeaderSize
	}

	fmt.Fprintf(c.w, `<defs><clipPath id="clip"><rect width="%d" height="%d"/></clipPath></defs>`,
		c.contentWidth(), c.contentHeight())

	fmt.Fprintf(c.w, `<g transform="translate(%d,%d)" clip-path="url(#clip)">`, Padding, contentY)

	c.writeStyles()
	c.writeBGFilters()

	// Animation group
	duration := c.rec.Duration.Seconds()
	loopAttr := "infinite"
	if c.config.LoopCount == -1 {
		loopAttr = "1"
	} else if c.config.LoopCount > 0 {
		loopAttr = fmt.Sprintf("%d", c.config.LoopCount)
	}

	fmt.Fprintf(c.w, `<g style="animation:k %.3fs %s steps(1,end)">`, duration, loopAttr)

	c.writeFrames()

	fmt.Fprint(c.w, `</g></g></svg>`)

	return nil
}

func (c *canvas) writeBackground() {
	bgHex := color.RGBAtoHex(c.config.Theme.WindowBackground)
	fmt.Fprintf(c.w, `<rect width="100%%" height="100%%" fill="%s"/>`, bgHex)
}

func (c *canvas) writeWindow() {
	theme := c.config.Theme

	// Window background with rounded corners
	bgHex := color.RGBAtoHex(theme.WindowBackground)
	fmt.Fprintf(c.w, `<rect rx="%d" width="100%%" height="100%%" fill="%s"/>`, windowCornerRadius, bgHex)

	// Window buttons (close, minimize, maximize)
	buttonY := Padding
	for i, btnColor := range theme.WindowButtons {
		btnHex := color.RGBAtoHex(btnColor)
		x := Padding + i*windowButtonSpacing
		fmt.Fprintf(c.w, `<circle cx="%d" cy="%d" r="%d" fill="%s"/>`, x, buttonY, windowButtonRadius, btnHex)
	}
}

func (c *canvas) writeStyles() {
	var sb strings.Builder
	sb.WriteString("<style>")

	// Keyframes animation
	sb.WriteString(c.generateKeyframes())

	// Cursor blink animation
	sb.WriteString("@keyframes blink{0%,50%{opacity:1}50.01%,100%{opacity:0}}")

	// Default text style (white-space:pre preserves spaces, survives minification)
	fgHex := color.RGBAtoHex(c.rec.Colors.DefaultForeground())
	fmt.Fprintf(&sb, "text{font-family:%s;font-size:%dpx;fill:%s;white-space:pre}",
		c.config.FontFamily, c.config.FontSize, fgHex)

	// Cursor style
	fmt.Fprintf(&sb, ".cursor{fill:%s;animation:blink 1s step-end infinite}", fgHex)

	// Color classes
	for id, rgba := range c.rec.Colors.All() {
		className := c.classNames[id]
		hex := color.RGBAtoHex(rgba)
		fmt.Fprintf(&sb, ".%s{fill:%s}", className, hex)
	}

	// Attribute classes (only if used)
	if c.rec.Stats.HasBold {
		sb.WriteString(".bold{font-weight:bold}")
	}
	if c.rec.Stats.HasItalic {
		sb.WriteString(".italic{font-style:italic}")
	}
	if c.rec.Stats.HasUnderline {
		sb.WriteString(".underline{text-decoration:underline}")
	}
	if c.rec.Stats.HasDim {
		sb.WriteString(".dim{opacity:0.5}")
	}

	sb.WriteString("</style>")
	fmt.Fprint(c.w, sb.String())
}

func (c *canvas) generateKeyframes() string {
	if len(c.rec.Frames) <= 1 {
		return "@keyframes k{0%{transform:translateX(0)}}"
	}

	var sb strings.Builder
	sb.WriteString("@keyframes k{")

	duration := c.rec.Duration.Seconds()
	pw := c.paddedWidth()

	for i, frame := range c.rec.Frames {
		pct := frame.Time.Seconds() / duration * 100
		offset := -pw * i
		fmt.Fprintf(&sb, "%.3f%%{transform:translateX(%dpx)}", pct, offset)
	}

	sb.WriteString("}")
	return sb.String()
}

func (c *canvas) writeBGFilters() {
	// Collect unique background colors used in frames
	bgColors := make(map[color.ID]bool)
	for _, frame := range c.rec.Frames {
		for _, row := range frame.Rows {
			for _, run := range row.Runs {
				if !c.rec.Colors.IsDefault(run.Attrs.BG) {
					bgColors[run.Attrs.BG] = true
				}
			}
		}
	}

	if len(bgColors) == 0 {
		return
	}

	fmt.Fprint(c.w, "<defs>")
	for id := range bgColors {
		rgba := c.rec.Colors.Resolved(id)
		hex := color.RGBAtoHex(rgba)
		fmt.Fprintf(c.w, `<filter id="bg_%d" x="0" y="0" width="1" height="1">`, id)
		fmt.Fprintf(c.w, `<feFlood flood-color="%s"/><feComposite in="SourceGraphic" operator="over"/>`, hex)
		fmt.Fprint(c.w, `</filter>`)
	}
	fmt.Fprint(c.w, "</defs>")
}

func (c *canvas) writeFrames() {
	pw := c.paddedWidth()
	for i, frame := range c.rec.Frames {
		offset := pw * i
		fmt.Fprintf(c.w, `<g transform="translate(%d,0)">`, offset)
		c.writeFrame(frame)
		fmt.Fprint(c.w, "</g>")
	}
}

func (c *canvas) writeFrame(frame ir.Frame) {
	for _, row := range frame.Rows {
		for _, run := range row.Runs {
			c.writeTextRun(run, row.Y)
		}
	}

	// Render cursor if visible
	if frame.Cursor.Visible {
		c.writeCursor(frame.Cursor)
	}
}

func (c *canvas) writeCursor(cursor ir.Cursor) {
	x := cursor.Col * ColWidth
	y := cursor.Row * RowHeight

	// Render cursor as a rectangle (block cursor)
	fmt.Fprintf(c.w, `<rect class="cursor" x="%d" y="%d" width="%d" height="%d"/>`,
		x, y, ColWidth, RowHeight)
}

func (c *canvas) writeTextRun(run ir.TextRun, rowY int) {
	if run.Text == "" {
		return
	}

	// Skip whitespace-only runs with default background - nothing visible to render
	if strings.TrimSpace(run.Text) == "" && c.rec.Colors.IsDefault(run.Attrs.BG) {
		return
	}

	// Replace spaces with non-breaking spaces to survive minification
	// Only needed when minifying, as the minifier strips regular spaces
	text := run.Text
	if c.config.Minify {
		text = strings.ReplaceAll(text, " ", "\u00A0")
	}

	x := run.StartCol * ColWidth
	y := (rowY*RowHeight + RowHeight) - 5 // baseline offset

	// Build class list
	var classes []string
	if !c.rec.Colors.IsDefault(run.Attrs.FG) {
		classes = append(classes, c.classNames[run.Attrs.FG])
	}
	if run.Attrs.Bold {
		classes = append(classes, "bold")
	}
	if run.Attrs.Italic {
		classes = append(classes, "italic")
	}
	if run.Attrs.Underline {
		classes = append(classes, "underline")
	}
	if run.Attrs.Dim {
		classes = append(classes, "dim")
	}

	// Build attributes
	classAttr := ""
	if len(classes) > 0 {
		classAttr = fmt.Sprintf(" class=%q", strings.Join(classes, " "))
	}

	filterAttr := ""
	if !c.rec.Colors.IsDefault(run.Attrs.BG) {
		filterAttr = fmt.Sprintf(` filter="url(#bg_%d)"`, run.Attrs.BG)
	}

	fmt.Fprintf(c.w, `<text x="%d" y="%d" xml:space="preserve"%s%s>%s</text>`,
		x, y, classAttr, filterAttr, html.EscapeString(text))
}
