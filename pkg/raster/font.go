package raster

import (
	_ "embed"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

//go:embed JetBrainsMono-Regular.ttf
var jetBrainsMonoTTF []byte

// loadFontFace loads the embedded JetBrains Mono font at the given size.
func loadFontFace(size float64) (font.Face, error) {
	f, err := opentype.Parse(jetBrainsMonoTTF)
	if err != nil {
		return nil, err
	}

	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}

	return face, nil
}
