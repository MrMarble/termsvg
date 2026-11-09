package color

import (
	"image/color"
	"testing"
)

func TestConstructors(t *testing.T) {
	tests := []struct {
		name string
		got  Color
		want Color
	}{
		{name: "ansi", got: FromANSI(3), want: Color{Type: ANSI, Index: 3}},
		{name: "extended", got: FromExtended(200), want: Color{Type: Extended, Index: 200}},
		{name: "rgb", got: FromRGB(1, 2, 3), want: Color{Type: TrueColor, RGB: color.RGBA{R: 1, G: 2, B: 3, A: 255}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %+v, want %+v", tt.got, tt.want)
			}
		})
	}
}

func TestColor_ToRGBA(t *testing.T) {
	palette := Standard()
	transparent := color.RGBA{}
	tests := []struct {
		name string
		in   Color
		want color.RGBA
	}{
		{name: "ansi uses palette", in: FromANSI(1), want: palette.At(1)},
		{name: "extended uses palette", in: FromExtended(200), want: palette.At(200)},
		{name: "truecolor is literal", in: FromRGB(10, 20, 30), want: color.RGBA{R: 10, G: 20, B: 30, A: 255}},
		{name: "default is transparent", in: Color{Type: Default}, want: transparent},
		{name: "unknown type is transparent", in: Color{Type: Type(99)}, want: transparent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.ToRGBA(&palette); got != tt.want {
				t.Errorf("ToRGBA() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestColor_ToHex(t *testing.T) {
	palette := Standard()
	tests := []struct {
		name string
		in   Color
		want string
	}{
		{name: "truecolor", in: FromRGB(0xFF, 0x57, 0x33), want: "#FF5733"},
		{name: "low values are zero padded", in: FromRGB(1, 2, 3), want: "#010203"},
		{name: "default", in: Color{Type: Default}, want: "#000000"},
		{name: "ansi", in: FromANSI(2), want: RGBAtoHex(palette.At(2))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.ToHex(&palette); got != tt.want {
				t.Errorf("ToHex() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRGBAtoHex_IgnoresAlpha(t *testing.T) {
	if got := RGBAtoHex(color.RGBA{R: 0xAB, G: 0xCD, B: 0xEF, A: 0x10}); got != "#ABCDEF" {
		t.Errorf("RGBAtoHex() = %q", got)
	}
}
