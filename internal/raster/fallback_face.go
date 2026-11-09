package raster

import (
	"image"
	"io"
	"os"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// fallbackFace is a font.Face that draws each rune with the first font that has
// a glyph for it. The primary font draws everything it can, exactly as it would
// alone; fallback glyphs are fitted to the primary font's cell so text stays on
// the terminal grid: their advance is the cell width, they are centered in the
// cell and, when too wide for it, scaled down.
//
// Runes that no font has are drawn by the primary font (its missing-glyph box).
// A fallbackFace is not safe for concurrent use.
type fallbackFace struct {
	size    float64
	primary *opentype.Font
	main    font.Face
	extras  []*extraFont
	cell    fixed.Int26_6 // advance of the primary font: one terminal cell
	buf     sfnt.Buffer
	plans   map[rune]*glyphPlan
	direct  *glyphPlan // the plan for runes the primary font draws as it is

	// sys, when set, is searched for runes none of the embedded fonts has.
	sys       *systemFonts
	sysExtras map[systemFontRef]*extraFont
}

// extraFont is a fallback font with the faces made from it. It is opened the
// first time a rune needs it. The nominal face is at the primary size; scaled
// faces are made for glyphs that must shrink.
type extraFont struct {
	open   func() (*opentype.Font, io.Closer, error)
	loaded bool
	broken bool
	font   *opentype.Font
	closer io.Closer
	face   font.Face
	scaled map[fixed.Int26_6]font.Face
}

// glyphPlan says how to draw one rune: with which face, and how far to move
// the glyph to center it in its cell.
type glyphPlan struct {
	face    font.Face
	shift   fixed.Int26_6
	primary bool // drawn by the primary face, untouched
}

// load opens the font and makes its nominal face, once. It reports whether the
// font is usable.
func (e *extraFont) load(size float64) bool {
	if e.loaded {
		return !e.broken
	}
	e.loaded = true
	f, closer, err := e.open()
	if err == nil {
		e.face, err = newOpenTypeFace(f, size)
		if err != nil && closer != nil {
			_ = closer.Close()
		}
	}
	if err != nil {
		e.broken = true
		return false
	}
	e.font, e.closer = f, closer
	e.scaled = make(map[fixed.Int26_6]font.Face)

	return true
}

func newFallbackFace(
	primary *opentype.Font, fallbacks [][]byte, sys *systemFonts, size float64,
) (*fallbackFace, error) {
	main, err := newOpenTypeFace(primary, size)
	if err != nil {
		return nil, err
	}

	f := &fallbackFace{
		size: size, primary: primary, main: main, plans: make(map[rune]*glyphPlan),
		direct: &glyphPlan{face: main, primary: true},
	}
	if cell, ok := main.GlyphAdvance('M'); ok {
		f.cell = cell
	}

	f.sys = sys
	for _, data := range fallbacks {
		f.extras = append(f.extras, &extraFont{open: func() (*opentype.Font, io.Closer, error) {
			otf, err := opentype.Parse(data)
			return otf, nil, err
		}})
	}

	return f, nil
}

// has reports whether the font has a glyph for r.
func (f *fallbackFace) has(otf *opentype.Font, r rune) bool {
	index, err := otf.GlyphIndex(&f.buf, r)
	return err == nil && index != 0
}

func (f *fallbackFace) plan(r rune) *glyphPlan {
	if r < utf8.RuneSelf {
		return f.direct // ASCII is always the primary font's
	}
	if p, ok := f.plans[r]; ok {
		return p
	}
	p := f.makePlan(r)
	f.plans[r] = p

	return p
}

func (f *fallbackFace) makePlan(r rune) *glyphPlan {
	if f.has(f.primary, r) {
		return f.direct
	}

	for _, extra := range f.extras {
		if p := f.planFrom(extra, r); p != nil {
			return p
		}
	}
	if f.sys != nil {
		for _, ref := range f.sys.candidates(r) {
			if p := f.planFrom(f.systemExtra(ref), r); p != nil {
				return p
			}
		}
	}

	return f.direct // no font has it: the primary font's missing-glyph box
}

// planFrom returns how to draw r with the given fallback font, or nil when the
// font cannot draw it (it has no glyph, or only a colour bitmap).
func (f *fallbackFace) planFrom(extra *extraFont, r rune) *glyphPlan {
	if !extra.load(f.size) || !f.has(extra.font, r) {
		return nil
	}

	face := extra.face
	bounds, _, ok := face.GlyphBounds(r)
	if !ok {
		return nil
	}
	if ink := bounds.Max.X - bounds.Min.X; f.cell > 0 && ink > f.cell {
		// Too wide for one cell: draw it smaller instead of overlapping.
		scaled, err := extra.scaledFace(f.size * float64(f.cell) / float64(ink))
		if err != nil {
			return nil
		}
		face = scaled
		bounds, _, _ = face.GlyphBounds(r)
	}

	return &glyphPlan{face: face, shift: (f.cell-(bounds.Max.X-bounds.Min.X))/2 - bounds.Min.X}
}

// systemExtra returns the fallback font for an installed face, creating it once.
func (f *fallbackFace) systemExtra(ref systemFontRef) *extraFont {
	if extra, ok := f.sysExtras[ref]; ok {
		return extra
	}
	if f.sysExtras == nil {
		f.sysExtras = make(map[systemFontRef]*extraFont)
	}
	extra := &extraFont{open: func() (*opentype.Font, io.Closer, error) { return openSystemFont(ref) }}
	f.sysExtras[ref] = extra

	return extra
}

// openSystemFont opens one face of an installed font. The file stays open until
// the returned closer is closed, because glyph data is read from it on demand.
func openSystemFont(ref systemFontRef) (*opentype.Font, io.Closer, error) {
	file, err := os.Open(ref.path)
	if err != nil {
		return nil, nil, err
	}

	var magic [4]byte
	_, _ = file.ReadAt(magic[:], 0)

	var otf *opentype.Font
	if string(magic[:]) != "ttcf" {
		otf, err = opentype.ParseReaderAt(file)
	} else {
		var collection *opentype.Collection
		if collection, err = opentype.ParseCollectionReaderAt(file); err == nil {
			otf, err = collection.Font(ref.face)
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	return otf, file, nil
}

// scaledFace returns a face of this font at the given size, creating it once.
func (e *extraFont) scaledFace(size float64) (font.Face, error) {
	key := fixed.Int26_6(size * 64)
	if face, ok := e.scaled[key]; ok {
		return face, nil
	}
	face, err := newOpenTypeFace(e.font, size)
	if err != nil {
		return nil, err
	}
	e.scaled[key] = face

	return face, nil
}

// Glyph implements font.Face.
func (f *fallbackFace) Glyph(dot fixed.Point26_6, r rune) (
	dr image.Rectangle, mask image.Image, maskp image.Point, advance fixed.Int26_6, ok bool,
) {
	p := f.plan(r)
	if p.primary {
		return f.main.Glyph(dot, r)
	}
	dot.X += p.shift
	dr, mask, maskp, _, ok = p.face.Glyph(dot, r)

	return dr, mask, maskp, f.cell, ok
}

// GlyphBounds implements font.Face.
func (f *fallbackFace) GlyphBounds(r rune) (bounds fixed.Rectangle26_6, advance fixed.Int26_6, ok bool) {
	p := f.plan(r)
	if p.primary {
		return f.main.GlyphBounds(r)
	}
	bounds, _, ok = p.face.GlyphBounds(r)
	bounds.Min.X += p.shift
	bounds.Max.X += p.shift

	return bounds, f.cell, ok
}

// GlyphAdvance implements font.Face.
func (f *fallbackFace) GlyphAdvance(r rune) (advance fixed.Int26_6, ok bool) {
	if f.plan(r).primary {
		return f.main.GlyphAdvance(r)
	}

	return f.cell, true
}

// Kern implements font.Face. Only glyphs of the primary font kern; fallback
// glyphs sit on the grid.
func (f *fallbackFace) Kern(r0, r1 rune) fixed.Int26_6 {
	if f.plan(r0).primary && f.plan(r1).primary {
		return f.main.Kern(r0, r1)
	}

	return 0
}

// Metrics implements font.Face.
func (f *fallbackFace) Metrics() font.Metrics { return f.main.Metrics() }

func (f *fallbackFace) Close() error {
	err := f.main.Close()
	closeExtra := func(extra *extraFont) {
		if !extra.loaded || extra.broken {
			return
		}
		_ = extra.face.Close()
		for _, face := range extra.scaled {
			_ = face.Close()
		}
		if extra.closer != nil {
			_ = extra.closer.Close()
		}
	}
	for _, extra := range f.extras {
		closeExtra(extra)
	}
	for _, extra := range f.sysExtras {
		closeExtra(extra)
	}

	return err
}
