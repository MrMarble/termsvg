package svg

import (
	"slices"
	"strings"
	"unicode"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/rivo/uniseg"
)

type visualGlyph struct {
	text     string
	startCol int
	width    int
	attrs    ir.CellAttrs
}

type visualCell struct {
	glyphIndex  int
	glyphOffset int
	visible     bool
	attrs       ir.CellAttrs
}

type visualRow struct {
	cells     []visualCell
	glyphs    []visualGlyph
	supported bool
	fallback  ir.Row
}

type visualGrid struct {
	width  int
	height int
	rows   []visualRow
}

func buildVisualGrid(width, height int, rows []ir.Row, colors *color.Catalog) visualGrid {
	grid := visualGrid{width: width, height: height, rows: make([]visualRow, max(0, height))}
	for y := range grid.rows {
		row := rowAt(rows, y)
		grid.rows[y] = buildVisualRow(row, width, colors)
	}
	return grid
}

func buildVisualRow(row ir.Row, width int, colors *color.Catalog) visualRow {
	fallback := row
	fallback.Runs = slices.Clone(row.Runs)
	result := visualRow{cells: make([]visualCell, max(0, width)), supported: width >= 0, fallback: fallback}
	for i := range result.cells {
		result.cells[i].glyphIndex = -1
	}
	if width < 0 {
		return result
	}

	occupied := make([]bool, width)
	for _, run := range row.Runs {
		glyphs, ok := visualRunGlyphs(run)
		if !ok || run.StartCol < 0 || run.EndCol > width {
			result.supported = false
			return result
		}
		for _, glyph := range glyphs {
			for col := glyph.startCol; col < glyph.startCol+glyph.width; col++ {
				if occupied[col] {
					result.supported = false
					return result
				}
				occupied[col] = true
			}
			index := len(result.glyphs)
			result.glyphs = append(result.glyphs, glyph)
			visible := glyph.text != " " || glyph.attrs.Underline || !colors.IsDefault(glyph.attrs.BG)
			for offset := range glyph.width {
				result.cells[glyph.startCol+offset] = visualCell{
					glyphIndex: index, glyphOffset: offset, visible: visible, attrs: glyph.attrs,
				}
			}
		}
	}
	return result
}

func visualRunGlyphs(run ir.TextRun) ([]visualGlyph, bool) {
	width := run.EndCol - run.StartCol
	if width <= 0 || run.Text == "" {
		return nil, width == 0 && run.Text == ""
	}
	if glyphs, ok := explicitVisualGlyphs(run, width); ok {
		return glyphs, true
	}
	if strings.IndexByte(run.Text, 0) >= 0 {
		return nil, false
	}

	segments := make([]string, 0, uniseg.GraphemeClusterCount(run.Text))
	iterator := uniseg.NewGraphemes(run.Text)
	for iterator.Next() {
		segments = append(segments, iterator.Str())
	}
	if len(segments) != 1 && len(segments) != width {
		return nil, false
	}
	glyphs := make([]visualGlyph, len(segments))
	col := run.StartCol
	for i, text := range segments {
		glyphWidth := 1
		if len(segments) == 1 {
			glyphWidth = width
		}
		glyphs[i] = visualGlyph{text: text, startCol: col, width: glyphWidth, attrs: run.Attrs}
		col += glyphWidth
	}
	return glyphs, true
}

func explicitVisualGlyphs(run ir.TextRun, width int) ([]visualGlyph, bool) {
	runes := []rune(run.Text)
	if len(runes) != width {
		return nil, false
	}
	glyphs := make([]visualGlyph, 0, width)
	for i, col := 0, run.StartCol; i < len(runes); i, col = i+1, col+1 {
		char := runes[i]
		if char == 0 || unicode.IsMark(char) {
			return nil, false
		}
		glyphWidth := 1
		if i+1 < len(runes) && runes[i+1] == 0 {
			glyphWidth = 2
			i++
		} else if char > 0x7f {
			return nil, false
		}
		glyphs = append(glyphs, visualGlyph{
			text: string(char), startCol: col, width: glyphWidth, attrs: run.Attrs,
		})
		col += glyphWidth - 1
	}
	return glyphs, true
}

func visualCellsEqual(a *visualRow, aCol int, b *visualRow, bCol int) bool {
	aCell, bCell := a.cells[aCol], b.cells[bCol]
	if !aCell.visible && !bCell.visible {
		return true
	}
	if aCell.visible != bCell.visible || aCell.attrs != bCell.attrs ||
		aCell.glyphOffset != bCell.glyphOffset || aCell.glyphIndex < 0 || bCell.glyphIndex < 0 {
		return false
	}
	aGlyph, bGlyph := a.glyphs[aCell.glyphIndex], b.glyphs[bCell.glyphIndex]
	return aGlyph.text == bGlyph.text && aGlyph.width == bGlyph.width
}
