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

type position struct {
	x   int
	y   int
	n   int
	raw int
}

type style struct {
	foregroundColor string
	backgroundColor string
	dim             bool
	bold            bool
	italic          bool
	underline       bool
	inverse         bool
	strikethrough   bool
}
type valueStruct struct {
	tag       string
	ansi      string
	decorator string
}
type chunk struct {
	kind     string
	value    valueStruct
	position position
	style    style
}

// ParsedAnsi ...
type ParsedAnsi struct {
	raw       string
	plainText string
	textArea  measuredText
	chunks    []chunk
}

type styleStack struct {
	foregroundColor []string
	backgroundColor []string
	boldDim         []string
}

func (s *styleStack) getForeGroundColor() *string {
	if len(s.foregroundColor) > 0 {
		return &s.foregroundColor[len(s.foregroundColor)-1]
	}
	return nil
}
func (s *styleStack) getBackGroundColor() *string {
	if len(s.backgroundColor) > 0 {
		return &s.backgroundColor[len(s.backgroundColor)-1]
	}
	return nil
}
func (s *styleStack) getDim() bool {
	return includes(s.boldDim, "dim")
}
func (s *styleStack) getBold() bool {
	return includes(s.boldDim, "bold")
}

type styleState struct {
	italic        bool
	underline     bool
	inverse       bool
	hidden        bool
	strikethrough bool
}

// MeasureTextArea returns {rows, colums} of given text
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

// Atomize split text into words by  sticky delimiters
func atomize(text string) ([]string, []string) {
	ansies := sliceUniq(stripansi.AnsiRegex.FindAllString(text, -1))
	words := superSplit(text, append(ansies, "\n"))
	return words, ansies
}

func bundle(kind string, value valueStruct, x, y, nAnsi, nPlain *int, styleStack *styleStack, styleState *styleState) chunk {
	chunk := chunk{kind: kind, value: value, position: position{x: *x, y: *y, n: *nPlain, raw: *nAnsi}}

	if kind == "text" || kind == "ansi" {
		style := style{}
		foregroundColor := styleStack.getForeGroundColor()
		backgroundColor := styleStack.getBackGroundColor()
		dim := styleStack.getDim()
		bold := styleStack.getBold()

		if foregroundColor != nil {
			style.foregroundColor = *foregroundColor
		}
		if backgroundColor != nil {
			style.backgroundColor = *foregroundColor
		}
		if dim {
			style.dim = true
		}
		if bold {
			style.bold = true
		}
		if styleState.italic {
			style.italic = true
		}
		if styleState.underline {
			style.underline = true
		}
		if styleState.inverse {
			style.inverse = true
		}
		if styleState.strikethrough {
			style.strikethrough = true
		}
	}
	return chunk
}

// Parse raw ansi
func Parse(ansi string) ParsedAnsi {
	plainText := stripansi.Strip(ansi)
	textArea := measureTextArea(plainText)
	words, ansies := atomize(ansi)

	var (
		x          int        = 0
		y          int        = 0
		nAnsi      int        = 0
		nPlain     int        = 0
		result     ParsedAnsi = ParsedAnsi{raw: ansi, plainText: plainText, textArea: textArea}
		styleStack styleStack = styleStack{
			foregroundColor: []string{},
			backgroundColor: []string{},
			boldDim:         []string{},
		}
		styleState styleState = styleState{
			hidden:        false,
			inverse:       false,
			italic:        false,
			strikethrough: false,
			underline:     false,
		}
	)

	for _, word := range words {
		// New line
		if word == "\n" {
			chunk := bundle("newLine", valueStruct{ansi: word}, &x, &y, &nAnsi, &nPlain, &styleStack, &styleState)
			result.chunks = append(result.chunks, chunk)
			x = 0
			y++
			nAnsi++
			nPlain++
			continue
		}

		// Text
		if !includes(ansies, word) {
			chunk := bundle("text", valueStruct{ansi: word}, &x, &y, &nAnsi, &nPlain, &styleStack, &styleState)
			result.chunks = append(result.chunks, chunk)

			wordWidth := runewidth.StringWidth(word)
			x += wordWidth
			nAnsi += wordWidth
			nPlain += wordWidth
			continue
		}

		// ANSI Escape characters
		ansiTag := AnsiSeqs[word]
		decorator := Decorators[ansiTag]
		color := ansiTag

		switch decorator {
		case "foregroundColorOpen":
			styleStack.foregroundColor = append(styleStack.foregroundColor, color)
			break
		case "foregroundColorClose":
			styleStack.foregroundColor = styleStack.foregroundColor[:len(styleStack.foregroundColor)]
			break
		case "backgroundColorOpen":
			styleStack.backgroundColor = append(styleStack.backgroundColor, color)
			break
		case "backgroundColorClose":
			styleStack.backgroundColor = styleStack.backgroundColor[:len(styleStack.backgroundColor)]
			break
		case "boldOpen":
			styleStack.boldDim = append(styleStack.boldDim, "bold")
			break
		case "dimOpen":
			styleStack.boldDim = append(styleStack.boldDim, "dim")
			break
		case "boldDimClose":
			styleStack.boldDim = styleStack.boldDim[:len(styleStack.boldDim)]
			break
		case "italicOpen":
			styleState.italic = true
			break
		case "italicClose":
			styleState.italic = false
			break
		case "underlineOpen":
			styleState.underline = true
			break
		case "underlineClose":
			styleState.underline = false
			break
		case "inverseOpen":
			styleState.inverse = true
			break
		case "inverseClose":
			styleState.inverse = false
			break
		case "strikethroughOpen":
			styleState.strikethrough = true
			break
		case "strikethroughClose":
			styleState.strikethrough = false
			break
		case "reset":
			styleState.strikethrough = false
			styleState.inverse = false
			styleState.italic = false
			styleStack.boldDim = []string{}
			styleStack.backgroundColor = []string{}
			styleStack.foregroundColor = []string{}
			break
		}

		chunk := bundle("ansi", valueStruct{tag: ansiTag, ansi: word, decorator: decorator}, &x, &y, &nAnsi, &nPlain, &styleStack, &styleState)
		result.chunks = append(result.chunks, chunk)
		nAnsi = runewidth.StringWidth(word)
	}
	return result
}
