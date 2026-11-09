package raster

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/layout"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
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
	face, err := loadFontFace(testFontSize, nil)
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
	x0 := layout.Padding
	y0 := layout.ContentOffsetY(r.config.ShowWindow) + rowY*layout.RowHeight
	for col := range cols {
		for x := x0 + col*layout.ColWidth; x < x0+(col+1)*layout.ColWidth; x++ {
			for y := y0; y < y0+layout.RowHeight; y++ {
				if img.RGBAAt(x, y) != bg {
					out[col] = true
				}
			}
		}
	}
	return out
}

func TestRasterize_FallbackGlyphsAreDrawnInsideTheirCells(t *testing.T) {
	cfg := testConfig()
	r, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}

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
	bg := frames[0].Image.RGBAAt(layout.Padding+0, layout.ContentOffsetY(r.config.ShowWindow)+layout.RowHeight*1)
	got := inkColumns(frames[0].Image, 8, 0, r, bg)
	want := []bool{false, false, true, false, true, false, true, false}
	for col := range want {
		if got[col] != want[col] {
			t.Errorf("column %d has ink = %v, want %v (a glyph leaked into a neighbouring cell, or was not drawn)",
				col, got[col], want[col])
		}
	}
}

// newFaceWith builds a face over JetBrains Mono with the given embedded
// fallbacks and system fonts, so a test can control exactly what is available.
func newFaceWith(t *testing.T, embedded [][]byte, sys *systemFonts) *fallbackFace {
	t.Helper()
	primary, err := opentype.Parse(jetBrainsMonoTTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := newFallbackFace(primary, embedded, sys, testFontSize)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = face.Close() })
	return face
}

func fontDir(t *testing.T, files map[string][]byte) *systemFonts {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		writeFile(t, filepath.Join(dir, name), data)
	}
	return newSystemFonts(nil, []string{dir})
}

// runesOnlyIn returns the first n runes (from U+0100 up) that the given font
// has and JetBrains Mono lacks, so a test never depends on a guess about either.
func runesOnlyIn(t *testing.T, data []byte, n int) []rune {
	t.Helper()
	other, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	primary, err := sfnt.Parse(jetBrainsMonoTTF)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	var runes []rune
	for r := rune(0x100); r < 0x3000 && len(runes) < n; r++ {
		a, _ := other.GlyphIndex(&buf, r)
		b, _ := primary.GlyphIndex(&buf, r)
		if a != 0 && b == 0 {
			runes = append(runes, r)
		}
	}
	if len(runes) < n {
		t.Fatalf("found only %d runes", len(runes))
	}
	return runes
}

func TestFallbackFace_SystemFontsFillWhatTheEmbeddedFontsLack(t *testing.T) {
	sys := fontDir(t, map[string][]byte{"DejaVuSansMono.ttf": dejaVuSansMonoTTF})
	without := newFaceWith(t, nil, nil)
	with := newFaceWith(t, nil, sys)
	tofu := drawGlyph(without, unassigned)

	for _, r := range runesOnlyIn(t, dejaVuSansMonoTTF, 3) {
		if without.has(without.primary, r) {
			t.Fatalf("%q is in the primary font: this test needs a rune it lacks", r)
		}
		// Without system fonts the rune is the missing-glyph box ...
		if g := drawGlyph(without, r); !g.samePixels(tofu) {
			t.Errorf("%q should be the missing-glyph box without system fonts", r)
		}
		// ... with them it is drawn, fitted to the cell like any fallback glyph.
		g := drawGlyph(with, r)
		if g.ink() == 0 || g.samePixels(tofu) {
			t.Errorf("%q is still the missing-glyph box with system fonts", r)
		}
		bounds, advance, ok := with.GlyphBounds(r)
		if !ok || advance != with.cell || bounds.Min.X < -16 || bounds.Max.X > with.cell+16 {
			t.Errorf("%q: bounds %v advance %v do not fit one cell (%v)", r, bounds, advance, with.cell)
		}
	}
}

func TestFallbackFace_EmbeddedFontsComeBeforeSystemFonts(t *testing.T) {
	// DejaVu is both embedded and installed: the embedded copy must be used, so
	// the flag never changes what the embedded fonts can already draw.
	sys := fontDir(t, map[string][]byte{"DejaVuSansMono.ttf": dejaVuSansMonoTTF})
	face := newFaceWith(t, [][]byte{dejaVuSansMonoTTF}, sys)

	only := runesOnlyIn(t, dejaVuSansMonoTTF, 3)
	if g := drawGlyph(face, only[0]); g.ink() == 0 {
		t.Fatal("nothing drawn")
	}
	if len(face.sysExtras) != 0 {
		t.Errorf("the system font was opened (%d) although an embedded font has the glyph", len(face.sysExtras))
	}

	// And the output is identical to a face with no system fonts at all.
	plain := newFaceWith(t, [][]byte{dejaVuSansMonoTTF}, nil)
	for _, r := range append(only, 'a', unassigned) {
		if a, b := drawGlyph(face, r), drawGlyph(plain, r); !a.samePixels(b) || a.rect != b.rect {
			t.Errorf("%q differs when system fonts are enabled", r)
		}
	}
}

func TestFallbackFace_UnusableSystemFontsAreSkipped(t *testing.T) {
	// Broken files, and fonts that do not have the rune, never get in the way.
	sys := fontDir(t, map[string][]byte{
		"a-broken.ttf":              []byte("not a font"),
		"b-JetBrainsMono-Mono.ttf":  jetBrainsMonoTTF, // does not have ȧ
		"c-DejaVuSansMono.ttf":      dejaVuSansMonoTTF,
		"d-truncated-Mono-Bold.ttf": dejaVuSansMonoTTF[:2000],
	})
	face := newFaceWith(t, nil, sys)
	if g := drawGlyph(face, 'ȧ'); g.ink() == 0 || g.samePixels(drawGlyph(face, unassigned)) {
		t.Error("ȧ was not drawn from the one usable font")
	}
}

func TestFallbackFace_ColourBitmapFontsCannotBeDrawn(t *testing.T) {
	const emoji = "/usr/share/fonts/noto/NotoColorEmoji.ttf"
	if _, err := os.Stat(emoji); err != nil {
		t.Skip("no colour emoji font installed")
	}
	dir := t.TempDir()
	if err := os.Symlink(emoji, filepath.Join(dir, "NotoColorEmoji.ttf")); err != nil {
		t.Skipf("cannot symlink: %v", err)
	}
	face := newFaceWith(t, nil, newSystemFonts(nil, []string{dir}))

	// The font has the glyph but only as a colour bitmap: the rune stays the
	// missing-glyph box instead of failing or drawing nothing.
	if !face.plan('😀').primary {
		t.Error("a colour bitmap glyph was accepted")
	}
	if g := drawGlyph(face, '😀'); g.ink() == 0 || g.advance != face.cell {
		t.Errorf("emoji: ink=%d advance=%v, want the primary font's box in one cell", g.ink(), g.advance)
	}
}

func TestFallbackFace_CloseReleasesSystemFontFiles(t *testing.T) {
	sys := fontDir(t, map[string][]byte{"DejaVuSansMono.ttf": dejaVuSansMonoTTF})
	primary, err := opentype.Parse(jetBrainsMonoTTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := newFallbackFace(primary, nil, sys, testFontSize)
	if err != nil {
		t.Fatal(err)
	}
	drawGlyph(face, 'ȧ')

	files := make([]*os.File, 0, len(face.sysExtras))
	for _, extra := range face.sysExtras {
		file, ok := extra.closer.(*os.File)
		if !ok {
			t.Fatalf("closer is %T, want an *os.File", extra.closer)
		}
		files = append(files, file)
	}
	if len(files) != 1 {
		t.Fatalf("%d system fonts open, want 1", len(files))
	}
	if _, err := files[0].Stat(); err != nil {
		t.Fatalf("the font file should be open while the face is in use: %v", err)
	}

	if err := face.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := files[0].Stat(); !errors.Is(err, os.ErrClosed) {
		t.Errorf("font file still open after Close: %v", err)
	}
}

func TestRasterizer_SystemFontsAreOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := testConfig()
		cfg.SystemFonts = enabled
		r, err := New(&cfg)
		if err != nil {
			t.Fatal(err)
		}
		if (r.sysFonts != nil) != enabled {
			t.Errorf("SystemFonts=%v but the rasterizer's system fonts are %v", enabled, r.sysFonts)
		}
	}
}

func TestRasterize_WithSystemFontsIsSafeAcrossWorkers(t *testing.T) {
	// Every worker builds its own face over the shared index; run under -race.
	t.Setenv(fontPathEnv, t.TempDir())
	cfg := testConfig()
	cfg.SystemFonts = true
	r, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}

	frames := make([]ir.Frame, 24)
	for i := range frames {
		frames[i] = ir.Frame{
			Delay: 10 * time.Millisecond,
			Rows:  []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "ȧ漢⠿😀" + string(rune('a'+i%26)), StartCol: 0}}}},
		}
		frames[i].Time = time.Duration(i) * 10 * time.Millisecond
	}
	rec := &ir.Recording{
		Width: 10, Height: 2, Frames: frames,
		Colors: termcolor.NewCatalog(color.RGBA{R: 192, G: 192, B: 192, A: 255}, color.RGBA{A: 255}),
	}
	got, err := r.Rasterize(rec)
	if err != nil || len(got) != len(frames) {
		t.Fatalf("Rasterize() = %d frames, %v", len(got), err)
	}
}
