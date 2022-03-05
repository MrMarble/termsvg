package color

import (
	"fmt"
	"image/color"
)

//nolint
var ansiColors = []color.Color{
	color.Black,                              // Black
	color.RGBA{0x00CD, 0x00, 0x00, 0x00},     // Red
	color.RGBA{0x00, 0x00CD, 0x00, 0x00},     // Green
	color.RGBA{0x00CD, 0x00CD, 0x00, 0x00},   // Yellow
	color.RGBA{0x00, 0x00, 0x00EE, 0x00},     // Blue
	color.RGBA{0x00CD, 0x00, 0x00CD, 0x00},   // Magent
	color.RGBA{0x00, 0x00CD, 0x00CD, 0x00},   // Cyan
	color.RGBA{0x00E5, 0x00E5, 0x00E5, 0x00}, // Grey

	color.RGBA{0x007F, 0x007F, 0x007F, 0x00}, // Dark Grey
	color.RGBA{0x00FF, 0x00, 0x00, 0x00},     // Light Red
	color.RGBA{0x00, 0x00FF, 0x00, 0x00},     // Light Green
	color.RGBA{0x00FF, 0x00FF, 0x00, 0x00},   // Light Yellow
	color.RGBA{0x005C, 0x005C, 0x00FF, 0x00}, // Light Blue
	color.RGBA{0x00FF, 0x00, 0x00FF, 0x00},   // Light Magent
	color.RGBA{0x00, 0x00FF, 0x00FF, 0x00},   // Light Cyan
	color.White,                              // White
}

func AnsiToColor(ansi uint32) color.Color {
	return ansiColors[ansi]
}

func ToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r), uint8(g), uint8(b))
}
