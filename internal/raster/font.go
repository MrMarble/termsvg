package raster

import (
	_ "embed"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// The embedded fonts and their licenses are described in fonts/README.md.

//go:embed fonts/JetBrainsMono-Regular.ttf
var jetBrainsMonoTTF []byte

//go:embed fonts/DejaVuSansMono.ttf
var dejaVuSansMonoTTF []byte

//go:embed fonts/NotoSansSymbols2-Regular.ttf
var notoSansSymbols2TTF []byte

// fallbackFonts are tried in order for runes the primary font lacks.
var fallbackFonts = [][]byte{dejaVuSansMonoTTF, notoSansSymbols2TTF}

// loadFontFace loads the embedded fonts at the given size: JetBrains Mono, with
// the fallback fonts for the glyphs it does not have. The fallback fonts are
// only parsed when a recording first needs one. The returned face is not safe
// for concurrent use; each goroutine needs its own.
func loadFontFace(size float64) (font.Face, error) {
	primary, err := opentype.Parse(jetBrainsMonoTTF)
	if err != nil {
		return nil, err
	}

	return newFallbackFace(primary, fallbackFonts, size)
}

func newOpenTypeFace(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}
