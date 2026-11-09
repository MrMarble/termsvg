package color

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// ParseHex parses a hex color string (e.g. "#ff5733", "ff5733" or "#f53")
// into an opaque color.RGBA.
func ParseHex(hex string) (color.RGBA, error) {
	hex = strings.TrimPrefix(hex, "#")

	switch len(hex) {
	case 3:
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case 6:
	default:
		return color.RGBA{}, fmt.Errorf("hex color must be 3 or 6 characters, got %d", len(hex))
	}

	var channels [3]uint8
	for i, name := range [...]string{"red", "green", "blue"} {
		v, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			return color.RGBA{}, fmt.Errorf("invalid %s component: %w", name, err)
		}
		channels[i] = uint8(v)
	}

	return color.RGBA{R: channels[0], G: channels[1], B: channels[2], A: 255}, nil
}
