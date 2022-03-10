package color

import (
	"github.com/hinshun/vt10x"
)

//go:generate go run colorsgen.go

func GetColor(c vt10x.Color) string {
	var colorStr string

	if c == vt10x.DefaultFG {
		return ansiColors[int(vt10x.LightGrey)]
	}

	if c.ANSI() {
		colorStr = ansiColors[int(c)]
	} else {
		colorStr = xtermColors[int(c)]
	}

	return colorStr
}
