package color

import (
	"github.com/hinshun/vt10x"
)

//go:generate go run colorsgen.go

func GetColor(c vt10x.Color) string {
	if c >= 1<<24 {
		return colors[int(vt10x.LightGrey)]
	}

	return colors[int(c)]
}
