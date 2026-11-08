package terminal

import (
	"testing"

	"github.com/hinshun/vt10x"
	"github.com/mrmarble/termsvg/internal/color"
)

var defaultColor = color.Color{Type: color.Default}

func write(t *testing.T, e *Emulator, s string) {
	t.Helper()
	if _, err := e.Write([]byte(s)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func TestNew(t *testing.T) {
	e := New(40, 10)
	if e.Width() != 40 || e.Height() != 10 {
		t.Errorf("size = %dx%d, want 40x10", e.Width(), e.Height())
	}
	var _ State = e
}

func TestEmulator_Write(t *testing.T) {
	e := New(20, 5)
	n, err := e.Write([]byte("hi"))
	if err != nil || n != 2 {
		t.Fatalf("Write() = %d, %v", n, err)
	}
	if got := e.Cell(0, 0).Char; got != 'h' {
		t.Errorf("cell(0,0) = %q, want 'h'", got)
	}
	if got := e.Cell(1, 0).Char; got != 'i' {
		t.Errorf("cell(1,0) = %q, want 'i'", got)
	}
}

func TestEmulator_Cursor(t *testing.T) {
	e := New(20, 5)
	write(t, e, "abc")
	if col, row := e.Cursor(); col != 3 || row != 0 {
		t.Errorf("cursor = (%d,%d), want (3,0)", col, row)
	}
	write(t, e, "\r\nx")
	if col, row := e.Cursor(); col != 1 || row != 1 {
		t.Errorf("cursor = (%d,%d), want (1,1)", col, row)
	}
}

func TestEmulator_CursorVisible(t *testing.T) {
	e := New(20, 5)
	if !e.CursorVisible() {
		t.Error("cursor should be visible by default")
	}
	write(t, e, "\x1b[?25l")
	if e.CursorVisible() {
		t.Error("cursor should be hidden after DECTCEM reset")
	}
	write(t, e, "\x1b[?25h")
	if !e.CursorVisible() {
		t.Error("cursor should be visible after DECTCEM set")
	}
}

func TestEmulator_Cell_Attributes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(Cell) bool
	}{
		{name: "bold", input: "\x1b[1mX", check: func(c Cell) bool { return c.Bold && !c.Italic && !c.Underline }},
		{name: "italic", input: "\x1b[3mX", check: func(c Cell) bool { return c.Italic && !c.Bold }},
		{name: "underline", input: "\x1b[4mX", check: func(c Cell) bool { return c.Underline && !c.Bold }},
		{name: "plain", input: "X", check: func(c Cell) bool { return !c.Bold && !c.Italic && !c.Underline }},
		{name: "reset", input: "\x1b[1m\x1b[0mX", check: func(c Cell) bool { return !c.Bold }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(10, 2)
			write(t, e, tt.input)
			c := e.Cell(0, 0)
			if c.Char != 'X' || !tt.check(c) {
				t.Errorf("cell = %+v", c)
			}
		})
	}
}

func TestEmulator_Cell_Colors(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantFG color.Color
		wantBG color.Color
	}{
		{name: "default", input: "X", wantFG: defaultColor, wantBG: defaultColor},
		{name: "ansi fg", input: "\x1b[31mX", wantFG: color.FromANSI(1), wantBG: defaultColor},
		{name: "ansi bg", input: "\x1b[42mX", wantFG: defaultColor, wantBG: color.FromANSI(2)},
		{name: "bright fg", input: "\x1b[91mX", wantFG: color.FromANSI(9), wantBG: defaultColor},
		{name: "extended fg", input: "\x1b[38;5;200mX", wantFG: color.FromExtended(200), wantBG: defaultColor},
		{name: "truecolor fg", input: "\x1b[38;2;10;20;30mX", wantFG: color.FromRGB(10, 20, 30), wantBG: defaultColor},
		{name: "truecolor bg", input: "\x1b[48;2;1;2;3mX", wantFG: defaultColor, wantBG: color.FromRGB(1, 2, 3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(10, 2)
			write(t, e, tt.input)
			c := e.Cell(0, 0)
			if c.Foreground != tt.wantFG {
				t.Errorf("FG = %+v, want %+v", c.Foreground, tt.wantFG)
			}
			if c.Background != tt.wantBG {
				t.Errorf("BG = %+v, want %+v", c.Background, tt.wantBG)
			}
		})
	}
}

func TestConvertColor(t *testing.T) {
	tests := []struct {
		name string
		in   vt10x.Color
		want color.Color
	}{
		{name: "default fg", in: vt10x.DefaultFG, want: color.Color{Type: color.Default}},
		{name: "default bg", in: vt10x.DefaultBG, want: color.Color{Type: color.Default}},
		{name: "ansi 0", in: 0, want: color.FromANSI(0)},
		{name: "ansi 15", in: 15, want: color.FromANSI(15)},
		{name: "extended 16", in: 16, want: color.FromExtended(16)},
		{name: "extended 255", in: 255, want: color.FromExtended(255)},
		{name: "rgb", in: vt10x.Color(0x0A141E), want: color.FromRGB(0x0A, 0x14, 0x1E)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertColor(tt.in); got != tt.want {
				t.Errorf("convertColor(%v) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}
