package theme

import (
	"image/color"
	"strings"
	"testing"
)

const testPalette = "#000000:#111111:#222222:#333333:#444444:#555555:#666666:#777777:" +
	"#888888:#999999:#aaaaaa:#bbbbbb:#cccccc:#dddddd:#eeeeee:#ffffff"

func TestDefault(t *testing.T) {
	th := Default()
	if th.Name != "default" {
		t.Errorf("Name = %q", th.Name)
	}
	if th.Background != (color.RGBA{A: 255}) {
		t.Errorf("Background = %v", th.Background)
	}
	if th.Foreground.A != 255 {
		t.Errorf("Foreground alpha = %d", th.Foreground.A)
	}
}

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
