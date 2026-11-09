# Embedded fonts

The raster renderers (GIF and WebM) draw text with these fonts, which are embedded in the binary.
SVG output does not use them: the viewer picks the font.

| File | Used for | Version | License |
| ---- | -------- | ------- | ------- |
| `JetBrainsMono-Regular.ttf` | Primary font. Every glyph it has is drawn with it. | 2.304 | SIL OFL 1.1, see `LICENSE-OFL-1.1.txt` |
| `DejaVuSansMono.ttf` | First fallback: Latin extensions (`ȧ`, `ẋ`), arrows, maths, geometric shapes and many symbols. | 2.37 | Bitstream Vera license (public domain changes), see `LICENSE-DejaVu.txt` |
| `NotoSansSymbols2-Regular.ttf` | Second fallback: Braille and further symbols. | 2.008 | SIL OFL 1.1, see `LICENSE-OFL-1.1.txt` |

The files are unmodified copies of the upstream fonts. A rune is drawn with the first font that has a glyph for
it; runes that none of them has are drawn as the primary font's missing-glyph box.

Not covered: CJK and other double-width characters, emoji, and the Powerline / Nerd Font private-use glyphs.
