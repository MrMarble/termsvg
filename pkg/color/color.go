package color

import (
	"github.com/hinshun/vt10x"
)

//go:generate go run colorsgen.go

func GetColor(c vt10x.Color) string {
	if c == vt10x.DefaultFG {
		return colors[int(vt10x.LightGrey)]
	}

	return colors[int(c)]
}
