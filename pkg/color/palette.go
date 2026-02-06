package color

import "image/color"

// Palette holds the 256 terminal colors.
type Palette [256]color.RGBA

// At returns the color at the given index.
func (p *Palette) At(index uint8) color.RGBA {
	return p[index]
}

// Standard returns the standard xterm 256-color palette.
func Standard() Palette {
	palette := Palette{
		// 0-15: Standard colors
		{0, 0, 0, 255},       // 0: Black
		{128, 0, 0, 255},     // 1: Red
		{0, 128, 0, 255},     // 2: Green
		{128, 128, 0, 255},   // 3: Yellow
		{0, 0, 128, 255},     // 4: Blue
		{128, 0, 128, 255},   // 5: Magenta
		{0, 128, 128, 255},   // 6: Cyan
		{192, 192, 192, 255}, // 7: White
		{128, 128, 128, 255}, // 8: Bright Black (Gray)
		{255, 0, 0, 255},     // 9: Bright Red
		{0, 255, 0, 255},     // 10: Bright Green
		{255, 255, 0, 255},   // 11: Bright Yellow
		{0, 0, 255, 255},     // 12: Bright Blue
		{255, 0, 255, 255},   // 13: Bright Magenta
		{0, 255, 255, 255},   // 14: Bright Cyan
		{255, 255, 255, 255}, // 15: Bright White
	}
	// 16-231: 6x6x6 Color Cube
	cubeValue := func(i int) uint8 {
		if i == 0 {
			return 0
		}
		return uint8(55 + i*40) //nolint:gosec // i is in range [1,5], result fits in uint8
	}
	idx := 16
	for r := range 6 {
		for g := range 6 {
			for b := range 6 {
				palette[idx] = color.RGBA{
					R: cubeValue(r),
					G: cubeValue(g),
					B: cubeValue(b),
					A: 255,
				}
				idx++
			}
		}
	}
	// 232-255: Grayscale Ramp
	for i := range 24 {
		gray := uint8(8 + i*10) //nolint:gosec // i is in range [0,23], result fits in uint8
		palette[idx] = color.RGBA{R: gray, G: gray, B: gray, A: 255}
		idx++
	}
	return palette
}
