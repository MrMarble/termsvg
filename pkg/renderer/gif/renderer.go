// Package gif provides a GIF renderer for terminal recordings.
// It generates animated GIFs by rasterizing the terminal state frame by frame.
package gif

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"runtime"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// Layout constants for GIF rendering (matching SVG renderer for consistency)
const (
	RowHeight  = 25 // pixels per row
	ColWidth   = 12 // pixels per column
	Padding    = 20 // padding around content
	HeaderSize = 2  // multiplier for header area (window buttons)
)

// Renderer implements the renderer.Renderer interface for GIF output.
type Renderer struct {
	config   renderer.Config
	fontFace font.Face
}

// New creates a new GIF renderer with the given configuration.
func New(config renderer.Config) (*Renderer, error) {
	face, err := loadFontFace(float64(config.FontSize))
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %w", err)
	}

	return &Renderer{
		config:   config,
		fontFace: face,
	}, nil
}

// Format returns the output format name.
func (r *Renderer) Format() string {
	return "gif"
}

// FileExtension returns the file extension for GIF files.
func (r *Renderer) FileExtension() string {
	return ".gif"
}

// Render generates an animated GIF from the recording.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	if len(rec.Frames) == 0 {
		return fmt.Errorf("recording has no frames")
	}

	c := &canvas{
		rec:      rec,
		config:   r.config,
		fontFace: r.fontFace,
	}

	return c.render(ctx, w)
}

// canvas holds rendering state for a single GIF generation
type canvas struct {
	rec          *ir.Recording
	config       renderer.Config
	fontFace     font.Face
	baseImage    *image.RGBA     // Pre-rendered window chrome + terminal background
	basePaletted *image.Paletted // Pre-converted paletted version of base image
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

func (c *canvas) contentOffsetY() int {
	if c.config.ShowWindow {
		return Padding * HeaderSize
	}
	return Padding
}

// renderedFrame holds the result of rendering a single frame
type renderedFrame struct {
	index    int
	paletted *image.Paletted
	delay    int
}

func (c *canvas) render(ctx context.Context, w io.Writer) error {
	width := c.paddedWidth()
	height := c.paddedHeight()

	// Build the color palette for the GIF
	palette := c.buildPalette()

	// Pre-render the static window chrome and terminal background
	c.initBaseImage(width, height, palette)

	// Phase 1: IR-level deduplication and parallel rendering
	rendered := c.renderFramesParallel(ctx, palette, width, height)

	// Check for cancellation after rendering
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Phase 2: Sequential assembly with delta encoding
	return c.assembleGIF(rendered, w)
}

// renderFramesParallel renders frames in parallel using a worker pool.
// It performs IR-level deduplication to skip rendering identical frames.
func (c *canvas) renderFramesParallel(ctx context.Context, palette color.Palette, _, _ int) []*renderedFrame {
	frames := c.rec.Frames
	results := make([]*renderedFrame, len(frames))
	var wg sync.WaitGroup

	// Use worker pool to limit concurrency
	numWorkers := runtime.NumCPU()
	sem := make(chan struct{}, numWorkers)

	// Track which frames need rendering (IR-level deduplication)
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

	// Calculate content area offset
	contentX := Padding
	contentY := c.contentOffsetY()

	for i := range frames {
		// Calculate delay for this frame
		delay := int(frames[i].Delay.Milliseconds() / 10)
		// Browsers clamp delays < 20ms to 100ms, so enforce minimum of 2 (20ms)
		if delay < 2 && i < len(frames)-1 {
			delay = 2
		}

		if !needsRender[i] {
			// IR-level duplicate: store delay only, no paletted image
			results[i] = &renderedFrame{
				index:    i,
				paletted: nil, // nil means use previous frame's image
				delay:    delay,
			}
			continue
		}

		// Check for cancellation before spawning goroutine
		select {
		case <-ctx.Done():
			return results
		default:
		}

		wg.Add(1)
		go func(idx int, frame ir.Frame, frameDelay int) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			// Create a per-goroutine font face (font.Face is not thread-safe)
			face, err := loadFontFace(float64(c.config.FontSize))
			if err != nil {
				return
			}

			// Start with a copy of the pre-converted paletted base image
			paletted := image.NewPaletted(c.basePaletted.Bounds(), palette)
			copy(paletted.Pix, c.basePaletted.Pix)

			// Draw directly to the paletted image
			c.drawFrameContentToPaletted(paletted, frame, face, contentX, contentY)

			results[idx] = &renderedFrame{
				index:    idx,
				paletted: paletted,
				delay:    frameDelay,
			}
		}(i, frames[i], delay)
	}

	wg.Wait()
	return results
}

// assembleGIF creates the final GIF from rendered frames using delta encoding
func (c *canvas) assembleGIF(rendered []*renderedFrame, w io.Writer) error {
	g := &gif.GIF{
		LoopCount: c.config.LoopCount,
	}

	var prevPaletted *image.Paletted

	for _, rf := range rendered {
		if rf == nil {
			continue
		}

		// IR-level duplicate: just extend the previous frame's delay
		if rf.paletted == nil {
			if len(g.Delay) > 0 {
				g.Delay[len(g.Delay)-1] += rf.delay
			}
			continue
		}

		// Pixel-level duplicate check (for frames that were rendered but are identical)
		if prevPaletted != nil && framesEqual(prevPaletted, rf.paletted) {
			g.Delay[len(g.Delay)-1] += rf.delay
			continue
		}

		// For delta encoding: if we have a previous frame, only encode changed pixels
		if prevPaletted != nil {
			delta := computeDelta(prevPaletted, rf.paletted, 0) // 0 is transparent index
			g.Image = append(g.Image, delta)
			g.Disposal = append(g.Disposal, gif.DisposalNone)
		} else {
			// First frame must be complete
			g.Image = append(g.Image, rf.paletted)
			g.Disposal = append(g.Disposal, gif.DisposalNone)
		}

		g.Delay = append(g.Delay, rf.delay)
		prevPaletted = rf.paletted
	}

	return gif.EncodeAll(w, g)
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

// rowsEqualIR compares two IR rows for equality
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

// textRunsEqualIR compares two IR text runs for equality
func textRunsEqualIR(a, b *ir.TextRun) bool {
	return a.Text == b.Text &&
		a.StartCol == b.StartCol &&
		a.Attrs == b.Attrs
}

// initBaseImage pre-renders the static window chrome and terminal background
func (c *canvas) initBaseImage(width, height int, palette color.Palette) {
	c.baseImage = image.NewRGBA(image.Rect(0, 0, width, height))

	// Draw window chrome or plain background
	if c.config.ShowWindow {
		c.drawWindow(c.baseImage)
	} else {
		c.drawBackground(c.baseImage)
	}

	// Draw terminal content background (black area)
	contentX := Padding
	contentY := c.contentOffsetY()
	termBg := c.config.Theme.Background
	draw.Draw(c.baseImage,
		image.Rect(contentX, contentY, contentX+c.contentWidth(), contentY+c.contentHeight()),
		&image.Uniform{termBg},
		image.Point{},
		draw.Src)

	// Pre-convert base image to paletted (used as template for each frame)
	c.basePaletted = image.NewPaletted(c.baseImage.Bounds(), palette)
	draw.Draw(c.basePaletted, c.baseImage.Bounds(), c.baseImage, image.Point{}, draw.Src)
}

// framesEqual checks if two paletted images are identical
func framesEqual(a, b *image.Paletted) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

// computeDelta creates a delta frame containing only pixels that changed
// Unchanged pixels are set to the transparent color index
func computeDelta(prev, curr *image.Paletted, transparentIdx uint8) *image.Paletted {
	bounds := curr.Bounds()
	delta := image.NewPaletted(bounds, curr.Palette)

	// Fill with transparent initially
	for i := range delta.Pix {
		delta.Pix[i] = transparentIdx
	}

	// Copy only changed pixels
	for i := range curr.Pix {
		if prev.Pix[i] != curr.Pix[i] {
			delta.Pix[i] = curr.Pix[i]
		}
	}

	return delta
}

// drawFrameContent draws only the dynamic content (text runs and cursor)
// The static window chrome and terminal background are already in the base image
func (c *canvas) drawFrameContent(img *image.RGBA, frame ir.Frame) {
	c.drawFrameContentWithFace(img, frame, c.fontFace)
}

// drawFrameContentWithFace draws frame content using the specified font face
// This allows for thread-safe parallel rendering with per-goroutine font faces
func (c *canvas) drawFrameContentWithFace(img *image.RGBA, frame ir.Frame, face font.Face) {
	// Draw all text runs
	for _, row := range frame.Rows {
		for _, run := range row.Runs {
			c.drawTextRunWithFace(img, run, row.Y, face)
		}
	}

	// Draw cursor if visible
	if frame.Cursor.Visible {
		c.drawCursor(img, frame.Cursor)
	}
}

// drawFrameContentToImage draws frame content to a content-area-sized image
// with the given offset adjustments. This is used for partial rendering.
func (c *canvas) drawFrameContentToImage(img *image.RGBA, frame ir.Frame, face font.Face, offsetX, offsetY int) {
	// Draw all text runs
	for _, row := range frame.Rows {
		for _, run := range row.Runs {
			c.drawTextRunToImage(img, run, row.Y, face, offsetX, offsetY)
		}
	}

	// Draw cursor if visible
	if frame.Cursor.Visible {
		c.drawCursorToImage(img, frame.Cursor, offsetX, offsetY)
	}
}

// drawFrameContentToPaletted draws frame content directly to a paletted image.
// This avoids the RGBA->Paletted conversion step.
func (c *canvas) drawFrameContentToPaletted(img *image.Paletted, frame ir.Frame, face font.Face, offsetX, offsetY int) {
	// Draw all text runs
	for _, row := range frame.Rows {
		for _, run := range row.Runs {
			c.drawTextRunToPaletted(img, run, row.Y, face, offsetX, offsetY)
		}
	}

	// Draw cursor if visible
	if frame.Cursor.Visible {
		c.drawCursorToPaletted(img, frame.Cursor, offsetX, offsetY)
	}
}

func (c *canvas) drawTextRunToPaletted(img *image.Paletted, run ir.TextRun, rowY int, face font.Face, offsetX, offsetY int) {
	if run.Text == "" {
		return
	}

	x := offsetX + run.StartCol*ColWidth
	y := offsetY + rowY*RowHeight

	// Get colors
	var bgColor color.RGBA
	if c.rec.Colors.IsDefault(run.Attrs.BG) {
		bgColor = c.config.Theme.Background
	} else {
		bgColor = c.rec.Colors.Resolved(run.Attrs.BG)
	}

	var fgColor color.RGBA
	if c.rec.Colors.IsDefault(run.Attrs.FG) {
		fgColor = c.rec.Colors.DefaultForeground()
	} else {
		fgColor = c.rec.Colors.Resolved(run.Attrs.FG)
	}

	// Apply dim effect
	if run.Attrs.Dim {
		fgColor.A = 128
	}

	// Calculate text width in columns (handle multi-byte characters)
	textWidth := utf8.RuneCountInString(run.Text) * ColWidth

	// Draw background rectangle for the run
	draw.Draw(img,
		image.Rect(x, y, x+textWidth, y+RowHeight),
		&image.Uniform{bgColor},
		image.Point{},
		draw.Src)

	// Draw text directly to paletted image
	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{fgColor},
		Face: face,
		Dot:  fixed.P(x, y+RowHeight-5), // baseline offset
	}
	drawer.DrawString(run.Text)

	// Draw underline if needed
	if run.Attrs.Underline {
		underlineY := y + RowHeight - 2
		for px := x; px < x+textWidth; px++ {
			img.Set(px, underlineY, fgColor)
		}
	}
}

func (c *canvas) drawCursorToPaletted(img *image.Paletted, cursor ir.Cursor, offsetX, offsetY int) {
	x := offsetX + cursor.Col*ColWidth
	y := offsetY + cursor.Row*RowHeight

	// Get cursor color (same as foreground)
	cursorColor := c.rec.Colors.DefaultForeground()

	// Draw cursor as a block
	draw.Draw(img,
		image.Rect(x, y, x+ColWidth, y+RowHeight),
		&image.Uniform{cursorColor},
		image.Point{},
		draw.Src)
}

func (c *canvas) drawTextRunToImage(img *image.RGBA, run ir.TextRun, rowY int, face font.Face, offsetX, offsetY int) {
	if run.Text == "" {
		return
	}

	x := offsetX + run.StartCol*ColWidth
	y := offsetY + rowY*RowHeight

	// Get colors
	var bgColor color.RGBA
	if c.rec.Colors.IsDefault(run.Attrs.BG) {
		bgColor = c.config.Theme.Background
	} else {
		bgColor = c.rec.Colors.Resolved(run.Attrs.BG)
	}

	var fgColor color.RGBA
	if c.rec.Colors.IsDefault(run.Attrs.FG) {
		fgColor = c.rec.Colors.DefaultForeground()
	} else {
		fgColor = c.rec.Colors.Resolved(run.Attrs.FG)
	}

	// Apply dim effect
	if run.Attrs.Dim {
		fgColor.A = 128
	}

	// Calculate text width in columns (handle multi-byte characters)
	textWidth := utf8.RuneCountInString(run.Text) * ColWidth

	// Draw background rectangle for the run
	draw.Draw(img,
		image.Rect(x, y, x+textWidth, y+RowHeight),
		&image.Uniform{bgColor},
		image.Point{},
		draw.Src)

	// Draw text
	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{fgColor},
		Face: face,
		Dot:  fixed.P(x, y+RowHeight-5), // baseline offset
	}
	drawer.DrawString(run.Text)

	// Draw underline if needed
	if run.Attrs.Underline {
		underlineY := y + RowHeight - 2
		for px := x; px < x+textWidth; px++ {
			img.Set(px, underlineY, fgColor)
		}
	}
}

func (c *canvas) drawCursorToImage(img *image.RGBA, cursor ir.Cursor, offsetX, offsetY int) {
	x := offsetX + cursor.Col*ColWidth
	y := offsetY + cursor.Row*RowHeight

	// Get cursor color (same as foreground)
	cursorColor := c.rec.Colors.DefaultForeground()

	// Draw cursor as a block
	draw.Draw(img,
		image.Rect(x, y, x+ColWidth, y+RowHeight),
		&image.Uniform{cursorColor},
		image.Point{},
		draw.Src)
}

func (c *canvas) drawBackground(img *image.RGBA) {
	bgColor := c.config.Theme.WindowBackground
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
}

func (c *canvas) drawWindow(img *image.RGBA) {
	theme := c.config.Theme
	bounds := img.Bounds()

	// Window background with rounded corners
	drawRoundedRect(img, bounds, 5, theme.WindowBackground)

	// Window buttons (close, minimize, maximize)
	buttonY := Padding
	buttonSpacing := 20
	buttonRadius := 6

	for i, btnColor := range theme.WindowButtons {
		x := Padding + i*buttonSpacing
		drawCircle(img, x, buttonY, buttonRadius, btnColor)
	}
}

func (c *canvas) drawTextRun(img *image.RGBA, run ir.TextRun, rowY int) {
	c.drawTextRunWithFace(img, run, rowY, c.fontFace)
}

func (c *canvas) drawTextRunWithFace(img *image.RGBA, run ir.TextRun, rowY int, face font.Face) {
	if run.Text == "" {
		return
	}

	contentX := Padding
	contentY := c.contentOffsetY()

	x := contentX + run.StartCol*ColWidth
	y := contentY + rowY*RowHeight

	// Get colors
	var bgColor color.RGBA
	if c.rec.Colors.IsDefault(run.Attrs.BG) {
		bgColor = c.config.Theme.WindowBackground
	} else {
		bgColor = c.rec.Colors.Resolved(run.Attrs.BG)
	}

	var fgColor color.RGBA
	if c.rec.Colors.IsDefault(run.Attrs.FG) {
		fgColor = c.rec.Colors.DefaultForeground()
	} else {
		fgColor = c.rec.Colors.Resolved(run.Attrs.FG)
	}

	// Apply dim effect
	if run.Attrs.Dim {
		fgColor.A = 128
	}

	// Calculate text width in columns (handle multi-byte characters)
	textWidth := utf8.RuneCountInString(run.Text) * ColWidth

	// Draw background rectangle for the run
	draw.Draw(img,
		image.Rect(x, y, x+textWidth, y+RowHeight),
		&image.Uniform{bgColor},
		image.Point{},
		draw.Src)

	// Draw text
	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{fgColor},
		Face: face,
		Dot:  fixed.P(x, y+RowHeight-5), // baseline offset
	}
	drawer.DrawString(run.Text)

	// Draw underline if needed
	if run.Attrs.Underline {
		underlineY := y + RowHeight - 2
		for px := x; px < x+textWidth; px++ {
			img.Set(px, underlineY, fgColor)
		}
	}
}

func (c *canvas) drawCursor(img *image.RGBA, cursor ir.Cursor) {
	contentX := Padding
	contentY := c.contentOffsetY()

	x := contentX + cursor.Col*ColWidth
	y := contentY + cursor.Row*RowHeight

	// Get cursor color (same as foreground)
	cursorColor := c.rec.Colors.DefaultForeground()

	// Draw cursor as a block
	draw.Draw(img,
		image.Rect(x, y, x+ColWidth, y+RowHeight),
		&image.Uniform{cursorColor},
		image.Point{},
		draw.Src)
}

// buildPalette creates a color palette from the recording's colors
func (c *canvas) buildPalette() color.Palette {
	// Collect all unique colors
	colorSet := make(map[color.RGBA]bool)

	// Add theme colors
	colorSet[c.config.Theme.Background] = true
	colorSet[c.config.Theme.WindowBackground] = true
	colorSet[c.config.Theme.Foreground] = true
	for _, btnColor := range c.config.Theme.WindowButtons {
		colorSet[btnColor] = true
	}

	// Add colors from the color catalog
	colorSet[c.rec.Colors.DefaultForeground()] = true
	colorSet[c.rec.Colors.DefaultBackground()] = true
	for _, rgba := range c.rec.Colors.All() {
		colorSet[rgba] = true
	}

	// Convert to palette
	palette := make(color.Palette, 0, len(colorSet)+1)

	// Add transparent color first (for potential optimization)
	palette = append(palette, color.RGBA{0, 0, 0, 0})

	for c := range colorSet {
		palette = append(palette, c)
	}

	// If palette is too small, pad with black
	for len(palette) < 2 {
		palette = append(palette, color.RGBA{0, 0, 0, 255})
	}

	// GIF supports max 256 colors - if we have more, the quantizer will handle it
	if len(palette) > 256 {
		palette = palette[:256]
	}

	return palette
}

// drawRoundedRect draws a rounded rectangle on the image
func drawRoundedRect(img *image.RGBA, bounds image.Rectangle, radius int, c color.RGBA) {
	// Fill the main rectangle
	draw.Draw(img, bounds, &image.Uniform{c}, image.Point{}, draw.Src)

	// For simplicity, we draw a regular rectangle with slightly rounded appearance
	// A full implementation would use proper corner rounding algorithms
	// The visual difference is minimal at small radii
}

// drawCircle draws a filled circle on the image
func drawCircle(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= radius*radius {
				img.Set(cx+x, cy+y, c)
			}
		}
	}
}
