package theme

import (
	"fmt"
	"image/color"
	"strings"

	termcolor "github.com/mrmarble/termsvg/internal/color"
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

// Default returns the default theme.
func Default() Theme {
	palette := termcolor.Standard()
	return Theme{
		Name:             "default",
		Palette:          palette,
		Foreground:       termcolor.FromANSI(7).ToRGBA(&palette),
		Background:       color.RGBA{R: 0, G: 0, B: 0, A: 255},
		WindowBackground: color.RGBA{R: 40, G: 45, B: 53, A: 255}, // #282d35
		WindowButtons: [3]color.RGBA{
			{R: 255, G: 95, B: 86, A: 255},  // Close - #FF5F58
			{R: 255, G: 189, B: 46, A: 255}, // Minimize - #FFBD2E
			{R: 24, G: 193, B: 50, A: 255},  // Maximize - #18c132
		},
	}
}

// FromAsciinema creates a Theme from asciinema format (fg, bg, palette string).
// The palette holds 8 or 16 colon-separated hex colors. When only 8 are given,
// the bright colors (8-15) repeat the normal ones (0-7), as the asciinema
// player does. Any other size is an error.
func FromAsciinema(name, fg, bg, palette string) (Theme, error) {
	// Parse foreground and background colors
	fgColor, err := termcolor.ParseHex(fg)
	if err != nil {
		return Theme{}, fmt.Errorf("invalid foreground color %q: %w", fg, err)
	}

	bgColor, err := termcolor.ParseHex(bg)
	if err != nil {
		return Theme{}, fmt.Errorf("invalid background color %q: %w", bg, err)
	}

	// Parse the palette: 8 or 16 colors
	paletteColors := strings.Split(palette, ":")
	if n := len(paletteColors); n != 8 && n != 16 {
		return Theme{}, fmt.Errorf("palette must have 8 or 16 colors, got %d", n)
	}

	// Build 256-color palette: start with standard, override first 16 colors
	fullPalette := termcolor.Standard()
	for i, hex := range paletteColors {
		c, err := termcolor.ParseHex(hex)
		if err != nil {
			return Theme{}, fmt.Errorf("invalid palette color %d %q: %w", i, hex, err)
		}
		fullPalette[i] = c
	}
	if len(paletteColors) == 8 {
		// Only the normal colors were given: the bright ones repeat them.
		copy(fullPalette[8:16], fullPalette[:8])
	}

	// Start with default theme and override asciinema-specific properties
	theme := Default()
	theme.Name = name
	theme.Palette = fullPalette
	theme.Foreground = fgColor
	theme.Background = bgColor
	theme.WindowBackground = bgColor // Use asciinema bg for window background too

	return theme, nil
}
