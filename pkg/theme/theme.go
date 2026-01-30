package theme

import (
	"image/color"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
)

// Theme defines the colorscheme for rendering.
type Theme struct {
	Name             string
	Palette          termcolor.Palette
	Foreground       color.RGBA    // Default text color
	Background       color.RGBA    // Default background color
	WindowBackground color.RGBA    // Window background color
	WindowButtons    [3]color.RGBA // Close, Minimize, Maximize button colors
}

func Default() Theme {
	return Theme{
		Name:             "default",
		Palette:          termcolor.Standard(),
		Foreground:       termcolor.FromANSI(7).ToRGBA(termcolor.Standard()),
		Background:       color.RGBA{R: 0, G: 0, B: 0, A: 255},
		WindowBackground: color.RGBA{R: 40, G: 45, B: 53, A: 255}, // #282d35
		WindowButtons: [3]color.RGBA{
			{R: 255, G: 95, B: 86, A: 255},  // Close - #FF5F58
			{R: 255, G: 189, B: 46, A: 255}, // Minimize - #FFBD2E
			{R: 24, G: 193, B: 50, A: 255},  // Maximize - #18c132
		},
	}
}
