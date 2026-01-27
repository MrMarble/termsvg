package theme

import (
	"image/color"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
)

// Theme defines the colorscheme for rendering.
type Theme struct {
	Name       string
	Palette    termcolor.Palette
	Foreground color.RGBA // Default text color
	Background color.RGBA // Default background color
}

func Default() Theme {
	return Theme{
		Name:       "default",
		Palette:    termcolor.Standard(),
		Foreground: color.RGBA{R: 192, G: 192, B: 192, A: 255},
		Background: color.RGBA{R: 0, G: 0, B: 0, A: 255},
	}
}
