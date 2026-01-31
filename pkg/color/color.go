package color

import (
	"fmt"
	"image/color"
)

// Type indicates how to interpret the color.
type Type uint8

// Color represents a terminal color.
type Color struct {
	Type  Type
	Index uint8      // For ANSI/Extended (0-255) colors
	RGB   color.RGBA // For TrueColor colors
}

const (
	Default Type = iota
	ANSI
	Extended
	TrueColor
)

func FromANSI(index uint8) Color {
	return Color{Type: ANSI, Index: index}
}

func FromExtended(index uint8) Color {
	return Color{Type: Extended, Index: index}
}

func FromRGB(r, g, b uint8) Color {
	return Color{Type: TrueColor, RGB: color.RGBA{R: r, G: g, B: b, A: 255}}
}

// ToRGBA converts the Color to an RGBA value using the palette.
func (c Color) ToRGBA(palette Palette) color.RGBA {
	switch c.Type {
	case ANSI, Extended:
		return palette.At(c.Index)
	case TrueColor:
		return c.RGB
	case Default:
		return color.RGBA{0, 0, 0, 0} // Transparent
	default:
		return color.RGBA{0, 0, 0, 0} // Transparent for unknown types
	}
}

// ToHex returns the color as a hex string (e.g., "#RRGGBB").
func (c Color) ToHex(palette Palette) string {
	rgba := c.ToRGBA(palette)
	return RGBAtoHex(rgba)
}

func RGBAtoHex(rgba color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", rgba.R, rgba.G, rgba.B)
}
