package color

import (
	"fmt"
	"image/color"

	"github.com/hinshun/vt10x"
)

//go:generate go run colorsgen.go

func GetColor(c vt10x.Color) string {
	switch {
	case c >= 1<<24:
		return colors[int(vt10x.LightGrey)]
	case c >= 1<<8:
		rgb := intToRGB(uint32(c))
		return fmt.Sprintf("#%02x%02x%02x", rgb.R, rgb.G, rgb.B)
	default:
		return colors[int(c)]
	}
}

func intToRGB(c uint32) color.RGBA {
	//nolint:gosec
	return color.RGBA{
		R: uint8(c >> 16),
		G: uint8(c >> 8),
		B: uint8(c),
		A: 255,
	}
}
