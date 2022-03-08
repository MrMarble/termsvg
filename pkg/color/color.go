package color

import (
	"github.com/hinshun/vt10x"
)

//nolint
var ansiColors = []string{
	"#000000", // Black
	"#800000", // Red
	"#008000", // Green
	"#808000", // Yellow
	"#000080", // Blue
	"#800080", // Magent
	"#008080", // Cyan
	"#c0c0c0", // Grey

	"#808080", // Dark Grey
	"#ff0000", // Light Red
	"#00ff00", // Light Green
	"#ffff00", // Light Yellow
	"#0000ff", // Light Blue
	"#ff00ff", // Light Magent
	"#00ffff", // Light Cyan
	"#ffffff", // White
}

func ansiToColor(ansi uint32) string {
	return ansiColors[ansi]
}

func GetColor(c vt10x.Color) string {
	var colorStr string

	if c.ANSI() {
		colorStr = ansiToColor(uint32(c))
	} else {
		colorStr = ansiToColor(uint32(vt10x.LightGrey))
	}

	return colorStr
}
