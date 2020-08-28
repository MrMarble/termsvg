package ansiparse

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

type measuredText struct {
	rows    int
	columns int
}

func sliceUniq(s []int) []int {
	for i := 0; i < len(s); i++ {
		for i2 := i + 1; i2 < len(s); i2++ {
			if s[i] == s[i2] {
				// delete
				s = append(s[:i2], s[i2+1:]...)
				i2--
			}
		}
	}
	return s
}

func measureTextArea(plainText string) measuredText {
	lines := strings.Split(plainText, "\n")
	rows := len(lines)

	colums := 0
	for _, line := range lines {
		len := runewidth.StringWidth(line)
		if len > colums {
			colums = len
		}
	}
	return measuredText{rows, colums}
}
