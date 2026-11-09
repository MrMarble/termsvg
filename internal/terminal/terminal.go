package terminal

import (
	"github.com/hinshun/vt10x"
	"github.com/mrmarble/termsvg/internal/color"
)

// Cell represents a single terminal cell
type Cell struct {
	Char       rune
	Foreground color.Color
	Background color.Color
	Bold       bool
	Italic     bool
	Underline  bool
	Dim        bool
}

// Attribute bits of vt10x's Glyph.Mode. vt10x keeps them unexported
// (attrReverse, attrUnderline, attrBold, attrGfx, attrItalic, attrBlink in
// state.go), so they are mirrored here; keep in sync when upgrading vt10x.
const (
	modeReverse   = 1 << 0
	modeUnderline = 1 << 1
	modeBold      = 1 << 2
	modeItalic    = 1 << 4
)

// Emulator wraps vt10x
type Emulator struct {
	term vt10x.Terminal
}

func New(width, height int) *Emulator {
	return &Emulator{
		term: vt10x.New(
			vt10x.WithSize(width, height),
		),
	}
}

func (e *Emulator) Write(data []byte) (int, error) {
	return e.term.Write(data)
}

// Resize changes the terminal size. Content that still fits is kept; sizes
// below 1 are ignored.
func (e *Emulator) Resize(width, height int) {
	e.term.Resize(width, height)
}

func (e *Emulator) Width() int {
	w, _ := e.term.Size()
	return w
}

func (e *Emulator) Height() int {
	_, h := e.term.Size()
	return h
}

func (e *Emulator) Cell(col, row int) Cell {
	c := e.term.Cell(col, row)
	return Cell{
		Char:       c.Char,
		Foreground: convertColor(c.FG),
		Background: convertColor(c.BG),
		Bold:       c.Mode&modeBold != 0,
		Italic:     c.Mode&modeItalic != 0,
		Underline:  c.Mode&modeUnderline != 0,
		Dim:        c.Mode&modeReverse != 0, // historical mapping: Dim tracks vt10x's reverse bit
	}
}

// Cursor returns the current cursor position (col, row)
func (e *Emulator) Cursor() (col, row int) {
	c := e.term.Cursor()
	return c.X, c.Y
}

func (e *Emulator) CursorVisible() bool {
	return e.term.CursorVisible()
}

func convertColor(c vt10x.Color) color.Color {
	switch {
	case c == vt10x.DefaultFG || c == vt10x.DefaultBG:
		return color.Color{Type: color.Default}
	case c < 16:
		return color.FromANSI(uint8(c & 0xFF))
	case c < 256:
		return color.FromExtended(uint8(c & 0xFF))
	default:
		r := uint8((c >> 16) & 0xFF)
		g := uint8((c >> 8) & 0xFF)
		b := uint8(c & 0xFF)
		return color.FromRGB(r, g, b)
	}
}
