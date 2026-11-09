package theme

import (
	"image/color"
	"strings"
	"testing"

	termcolor "github.com/mrmarble/termsvg/internal/color"
)

const testPalette = "#000000:#111111:#222222:#333333:#444444:#555555:#666666:#777777:" +
	"#888888:#999999:#aaaaaa:#bbbbbb:#cccccc:#dddddd:#eeeeee:#ffffff"

const testPalette8 = "#000000:#cc0000:#00cc00:#cccc00:#0000cc:#cc00cc:#00cccc:#cccccc"

func TestFromAsciinema(t *testing.T) {
	tests := []struct {
		name    string
		fg, bg  string
		palette string
		wantErr string
	}{
		{name: "valid", fg: "#ffffff", bg: "#000000", palette: testPalette},
		{name: "short hex", fg: "#fff", bg: "#000", palette: testPalette},
		{name: "bad fg", fg: "zzz", bg: "#000", palette: testPalette, wantErr: "foreground"},
		{name: "bad bg", fg: "#fff", bg: "12", palette: testPalette, wantErr: "background"},
		{name: "wrong palette size", fg: "#fff", bg: "#000", palette: "#000:#fff", wantErr: "16 colors"},
		{
			name: "bad palette color", fg: "#fff", bg: "#000",
			palette: strings.Replace(testPalette, "#111111", "#11111g", 1), wantErr: "palette color 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th, err := FromAsciinema("mine", tt.fg, tt.bg, tt.palette)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if th.Name != "mine" || th.WindowBackground != th.Background {
				t.Errorf("theme = %+v", th)
			}
			if th.Palette[1] != (color.RGBA{R: 0x11, G: 0x11, B: 0x11, A: 255}) {
				t.Errorf("Palette[1] = %v", th.Palette[1])
			}
		})
	}
}

func TestFromAsciinema_EightColorPalette(t *testing.T) {
	th, err := FromAsciinema("eight", "#fff", "#000", testPalette8)
	if err != nil {
		t.Fatalf("FromAsciinema() error = %v", err)
	}
	for i := range 8 {
		if th.Palette[i] != th.Palette[i+8] {
			t.Errorf("bright color %d = %v, want a copy of color %d (%v)", i+8, th.Palette[i+8], i, th.Palette[i])
		}
	}
	if want := (color.RGBA{R: 0xcc, A: 255}); th.Palette[1] != want || th.Palette[9] != want {
		t.Errorf("Palette[1]/[9] = %v/%v, want %v", th.Palette[1], th.Palette[9], want)
	}
	// Colors beyond the 16 ANSI ones keep the standard 256-color values.
	if std := termcolor.Standard(); th.Palette[100] != std[100] {
		t.Errorf("Palette[100] = %v, want the standard %v", th.Palette[100], std[100])
	}
}

func TestFromAsciinema_PaletteSizes(t *testing.T) {
	colors := func(n int) string {
		return strings.TrimSuffix(strings.Repeat("#112233:", n), ":")
	}
	for n := 0; n <= 18; n++ {
		_, err := FromAsciinema("x", "#fff", "#000", colors(n))
		valid := n == 8 || n == 16
		if valid && err != nil {
			t.Errorf("%d colors: unexpected error %v", n, err)
		}
		if !valid && (err == nil || !strings.Contains(err.Error(), "8 or 16 colors")) {
			t.Errorf("%d colors: error = %v, want '8 or 16 colors'", n, err)
		}
	}
}
