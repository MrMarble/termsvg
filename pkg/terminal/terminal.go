package terminal

import (
	"github.com/hinshun/vt10x"
	"github.com/mrmarble/termsvg/pkg/color"
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

// State represents terminal state at a given time
type State interface {
	Width() int
	Height() int
	Cell(col, row int) Cell
}

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

// Write writes data to the terminal emulator
func (e *Emulator) Write(data []byte) (int, error) {
	return e.term.Write(data)
}

// Width returns terminal width
func (e *Emulator) Width() int {
	w, _ := e.term.Size()
	return w
}

// Height returns terminal height
func (e *Emulator) Height() int {
	_, h := e.term.Size()
	return h
}

// Cell returns the cell at the given column and row
func (e *Emulator) Cell(col, row int) Cell {
	c := e.term.Cell(col, row)
	return Cell{
		Char:       c.Char,
		Foreground: convertColor(c.FG),
		Background: convertColor(c.BG),
		Bold:       c.Mode&4 != 0,
		Italic:     c.Mode&16 != 0,
		Underline:  c.Mode&2 != 0,
		Dim:        c.Mode&1 != 0,
	}
}

// Cursor returns the current cursor position (col, row)
func (e *Emulator) Cursor() (col, row int) {
	c := e.term.Cursor()
	return c.X, c.Y
}

// CursorVisible returns whether the cursor is visible
func (e *Emulator) CursorVisible() bool {
	return e.term.CursorVisible()
}

// convertColor translates vt10x.Color to color.Color
func convertColor(c vt10x.Color) color.Color {
	switch {
	case c == vt10x.DefaultFG || c == vt10x.DefaultBG:
		return color.Color{Type: color.Default}
	case c < 16:
		return color.FromANSI(uint8(c & 0xFF)) //nolint:gosec // c is already < 16, safe to convert
	case c < 256:
		return color.FromExtended(uint8(c & 0xFF)) //nolint:gosec // c is already < 256, safe to convert
	default:
		r := uint8((c >> 16) & 0xFF) //nolint:gosec // masking ensures value fits in uint8
		g := uint8((c >> 8) & 0xFF)  //nolint:gosec // masking ensures value fits in uint8
		b := uint8(c & 0xFF)         //nolint:gosec // masking ensures value fits in uint8
		return color.FromRGB(r, g, b)
	}
}
