package raster

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// glyphImage is what a face draws for a rune with the dot at (originX, 40).
type glyphImage struct {
	alpha   *image.Alpha
	rect    image.Rectangle // bounds of the drawn glyph
	advance fixed.Int26_6
	ok      bool
}

const (
	testFontSize = 20
	// unassigned is a private-use rune that none of the embedded fonts has.
	unassigned = '\U0010FFFD'
	// originX is where drawGlyph puts the glyph's dot.
	originX = 20
)

func newTestFallbackFace(t *testing.T) *fallbackFace {
	t.Helper()
	face, err := loadFontFace(testFontSize)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = face.Close() })
	ff, ok := face.(*fallbackFace)
	if !ok {
		t.Fatalf("loadFontFace returned %T, want *fallbackFace", face)
	}
	return ff
}

func newPrimaryOnlyFace(t *testing.T) font.Face {
	t.Helper()
	otf, err := opentype.Parse(jetBrainsMonoTTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := newOpenTypeFace(otf, testFontSize)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = face.Close() })
	return face
}

// drawGlyph draws a rune the way font.Drawer does: whenever the glyph rectangle
// is not empty, whatever the ok flag says. That is how a font's missing-glyph
// box ends up on screen.
func drawGlyph(face font.Face, r rune) glyphImage {
	dr, mask, maskp, advance, ok := face.Glyph(fixed.P(originX, 40), r)
	canvas := image.NewAlpha(image.Rect(0, 0, 80, 60))
	if !dr.Empty() {
		draw.DrawMask(canvas, dr, image.Opaque, image.Point{}, mask, maskp, draw.Over)
	}
	return glyphImage{alpha: canvas, rect: dr, advance: advance, ok: ok}
}

func (g glyphImage) ink() int {
	n := 0
	for _, a := range g.alpha.Pix {
		if a != 0 {
			n++
		}
	}
	return n
}

func (g glyphImage) samePixels(other glyphImage) bool {
	if len(g.alpha.Pix) != len(other.alpha.Pix) {
		return false
	}
	for i := range g.alpha.Pix {
		if g.alpha.Pix[i] != other.alpha.Pix[i] {
			return false
		}
	}
	return true
}

func TestFallbackFace_DrawsGlyphsThePrimaryFontLacks(t *testing.T) {
	ff := newTestFallbackFace(t)
	tofu := drawGlyph(ff, unassigned)
	if tofu.ink() == 0 {
		t.Fatal("the missing-glyph reference draws nothing")
	}

	// Every one of these is absent from JetBrains Mono.
	for _, r := range []rune{'ȧ', 'ẋ', '⮠', '⠁', '⠿', '⣿', '⌛'} {
		t.Run(string(r), func(t *testing.T) {
			if ff.has(ff.primary, r) {
				t.Fatalf("%q is in the primary font: this test needs a rune it lacks", r)
			}
			g := drawGlyph(ff, r)
			if g.ink() == 0 {
				t.Fatalf("nothing was drawn for %q", r)
			}
			if g.samePixels(tofu) {
				t.Errorf("%q is drawn as the missing-glyph box", r)
			}
		})
	}
}

func TestFallbackFace_FallbackGlyphsAreFittedToTheirCell(t *testing.T) {
	ff := newTestFallbackFace(t)
	const tolerance = fixed.Int26_6(16) // a quarter of a pixel
	// ⮠ is wider than a cell in its own font (0.65 em against 0.6 em) and must be
	// scaled down; the others are narrower and must be centered.
	for _, r := range []rune{'ȧ', 'ẋ', '⮠', '⠁', '⠿', '⣿', '⌛', '✔'} {
		t.Run(string(r), func(t *testing.T) {
			if ff.has(ff.primary, r) {
				t.Skipf("%q is in the primary font", r)
			}
			bounds, advance, ok := ff.GlyphBounds(r)
			if !ok || advance != ff.cell {
				t.Fatalf("GlyphBounds ok=%v advance=%v, want one cell (%v)", ok, advance, ff.cell)
			}
			if bounds.Min.X < -tolerance || bounds.Max.X > ff.cell+tolerance {
				t.Errorf("ink spans [%v, %v], want it inside the cell [0, %v]", bounds.Min.X, bounds.Max.X, ff.cell)
			}
			if offset := (bounds.Min.X+bounds.Max.X)/2 - ff.cell/2; offset < -tolerance || offset > tolerance {
				t.Errorf("ink is %v off the cell's center, want it centered", offset)
			}
			if adv, ok := ff.GlyphAdvance(r); !ok || adv != ff.cell {
				t.Errorf("GlyphAdvance = %v,%v, want one cell", adv, ok)
			}
			// What is actually drawn agrees with the reported bounds.
			g := drawGlyph(ff, r)
			cell := int(ff.cell >> 6)
			if g.advance != ff.cell || g.rect.Min.X < originX-1 || g.rect.Max.X > originX+cell+1 {
				t.Errorf("drew x=[%d,%d) advance=%v, want within [%d,%d) and one cell advance",
					g.rect.Min.X, g.rect.Max.X, g.advance, originX, originX+cell)
			}
		})
	}
}

func TestFallbackFace_PrimaryGlyphsAreUnchanged(t *testing.T) {
	ff := newTestFallbackFace(t)
	plain := newPrimaryOnlyFace(t)

	runes := make([]rune, 0, 0x7f-0x20+0x25a0-0x2500+8)
	for r := rune(0x20); r < 0x7f; r++ { // ASCII
		runes = append(runes, r)
	}
	for r := rune(0x2500); r <= 0x259f; r++ { // box drawing and block elements
		runes = append(runes, r)
	}
	runes = append(runes, 'é', 'ñ', 'ü', '→', '✗', '➜', '•', '…')

	for _, r := range runes {
		if !ff.has(ff.primary, r) {
			continue // not a primary glyph; covered by the other tests
		}
		got, want := drawGlyph(ff, r), drawGlyph(plain, r)
		if got.advance != want.advance || got.rect != want.rect || !got.samePixels(want) {
			t.Fatalf("%q (U+%04X) is not drawn exactly as by the primary font alone", r, r)
		}
		if a, b := ff.Kern(r, 'a'), plain.Kern(r, 'a'); a != b {
			t.Fatalf("Kern(%q, 'a') = %v, want %v", r, a, b)
		}
	}
}

func TestFallbackFace_RunesNoFontHasAreTheMissingGlyphBox(t *testing.T) {
	ff := newTestFallbackFace(t)
	plain := newPrimaryOnlyFace(t)
	for _, r := range []rune{unassigned, '漢', '字', '😀', ''} {
		if !ff.plan(r).primary {
			t.Skipf("%q is covered by a fallback font", r)
		}
		got, want := drawGlyph(ff, r), drawGlyph(plain, r)
		if got.ink() == 0 || !got.samePixels(want) || got.advance != want.advance {
			t.Errorf("%q: want the primary font's missing-glyph box", r)
		}
	}
}

func TestFallbackFace_KernDoesNotApplyToFallbackGlyphs(t *testing.T) {
	ff := newTestFallbackFace(t)
	if got := ff.Kern('⠿', 'a'); got != 0 {
		t.Errorf("Kern with a fallback glyph = %v, want 0", got)
	}
	if got := ff.Kern('a', 'ẋ'); got != 0 {
		t.Errorf("Kern with a fallback glyph = %v, want 0", got)
	}
}

func TestFallbackFace_MetricsComeFromThePrimaryFont(t *testing.T) {
	ff := newTestFallbackFace(t)
	if got, want := ff.Metrics(), newPrimaryOnlyFace(t).Metrics(); got != want {
		t.Errorf("Metrics = %+v, want the primary font's %+v", got, want)
	}
	if ff.cell <= 0 {
		t.Errorf("cell width = %v", ff.cell)
	}
}

// ink reports, for each cell column of the row at rowY, whether it holds any
// pixel that differs from the background.
func inkColumns(img *image.RGBA, cols, rowY int, r *Rasterizer, bg color.RGBA) []bool {
	out := make([]bool, cols)
	x0 := r.config.Padding
	y0 := r.contentOffsetY() + rowY*r.config.RowHeight
	for col := range cols {
		for x := x0 + col*r.config.ColWidth; x < x0+(col+1)*r.config.ColWidth; x++ {
			for y := y0; y < y0+r.config.RowHeight; y++ {
				if img.RGBAAt(x, y) != bg {
					out[col] = true
				}
			}
		}
	}
	return out
}

func TestRasterize_FallbackGlyphsAreDrawnInsideTheirCells(t *testing.T) {
	cfg := DefaultConfig()
	r, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	rec := &ir.Recording{
		Width: 8, Height: 2,
		Colors: termcolor.NewCatalog(color.RGBA{R: 192, G: 192, B: 192, A: 255}, color.RGBA{A: 255}),
		Frames: []ir.Frame{{Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{
			{Text: "⠿", StartCol: 2}, // Braille: not in the primary font
			{Text: "ẋ", StartCol: 4},
			{Text: "✔", StartCol: 6}, // wider than a cell in its font
		}}}}},
	}
	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatal(err)
	}

	// The terminal background is what an empty cell looks like.
	bg := frames[0].Image.RGBAAt(r.config.Padding+0, r.contentOffsetY()+r.config.RowHeight*1)
	got := inkColumns(frames[0].Image, 8, 0, r, bg)
	want := []bool{false, false, true, false, true, false, true, false}
	for col := range want {
		if got[col] != want[col] {
			t.Errorf("column %d has ink = %v, want %v (a glyph leaked into a neighbouring cell, or was not drawn)",
				col, got[col], want[col])
		}
	}
}
