package color

import (
	"image/color"
	"testing"
)

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

func TestRGBAtoHex_IgnoresAlpha(t *testing.T) {
	if got := RGBAtoHex(color.RGBA{R: 0xAB, G: 0xCD, B: 0xEF, A: 0x10}); got != "#ABCDEF" {
		t.Errorf("RGBAtoHex() = %q", got)
	}
}
