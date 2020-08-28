package ansiparse

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/mrmarble/termsvg/stripansi"
)

type measuredText struct {
	rows    int
	columns int
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

func atomize(text string) struct {
	words  []string
	ansies []string
} {
	ansies := sliceUniq(stripansi.AnsiRegex.FindAllString(text, -1))
	words := superSplit(text, append(ansies, "\n"))
	return struct {
		words  []string
		ansies []string
	}{words, ansies}
}
