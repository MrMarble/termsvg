package svg

import (
	"reflect"
	"testing"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

func TestBuildVisualGridRepresentsExactGlyphExtents(t *testing.T) {
	rows := []ir.Row{
		{Y: 0, Runs: []ir.TextRun{{Text: "ABC", StartCol: 0, EndCol: 3}}},
		{Y: 1, Runs: []ir.TextRun{{Text: "e\u0301", StartCol: 0, EndCol: 1}}},
		{Y: 2, Runs: []ir.TextRun{{Text: "界", StartCol: 0, EndCol: 2}}},
		{Y: 3, Runs: []ir.TextRun{{Text: "✈️", StartCol: 0, EndCol: 2}}},
		{Y: 4, Runs: []ir.TextRun{{Text: "界\x00A", StartCol: 0, EndCol: 3}}},
		{Y: 5, Runs: []ir.TextRun{{Text: "x", StartCol: 1, EndCol: 4}}},
		{Y: 6, Runs: []ir.TextRun{{Text: "A\x00", StartCol: 0, EndCol: 2}}},
	}
	wantRows := cloneRows(rows)

	grid := buildVisualGrid(4, 7, rows, testCatalog())
	if grid.width != 4 || grid.height != 7 {
		t.Fatalf("grid dimensions = %dx%d", grid.width, grid.height)
	}
	for y, row := range grid.rows {
		if !row.supported {
			t.Fatalf("row %d unexpectedly unsupported: %#v", y, row.fallback)
		}
	}
	if got := glyphs(&grid.rows[0]); !reflect.DeepEqual(got, []visualGlyph{
		{text: "A", startCol: 0, width: 1},
		{text: "B", startCol: 1, width: 1},
		{text: "C", startCol: 2, width: 1},
	}) {
		t.Fatalf("ASCII glyphs = %#v", got)
	}
	for y, want := range []visualGlyph{
		{text: "e\u0301", startCol: 0, width: 1},
		{text: "界", startCol: 0, width: 2},
		{text: "✈️", startCol: 0, width: 2},
	} {
		if got := grid.rows[y+1].glyphs[0]; got != want {
			t.Fatalf("row %d glyph = %#v, want %#v", y+1, got, want)
		}
	}
	if got := glyphs(&grid.rows[4]); !reflect.DeepEqual(got, []visualGlyph{
		{text: "界", startCol: 0, width: 2},
		{text: "A", startCol: 2, width: 1},
	}) {
		t.Fatalf("NUL continuation glyphs = %#v", got)
	}
	if got := grid.rows[5].glyphs[0]; got.text != "x" || got.startCol != 1 || got.width != 3 {
		t.Fatalf("explicit oversized extent glyph = %#v", got)
	}
	if got := grid.rows[6].glyphs[0]; got.text != "A" || got.width != 2 {
		t.Fatalf("ASCII NUL continuation glyph = %#v", got)
	}
	for _, tc := range []struct{ y, col, glyph, offset int }{
		{2, 0, 0, 0}, {2, 1, 0, 1}, {4, 0, 0, 0}, {4, 1, 0, 1}, {4, 2, 1, 0},
	} {
		cell := grid.rows[tc.y].cells[tc.col]
		if cell.glyphIndex != tc.glyph || cell.glyphOffset != tc.offset {
			t.Fatalf("cell [%d,%d] = %#v", tc.y, tc.col, cell)
		}
	}
	if !reflect.DeepEqual(rows, wantRows) {
		t.Fatalf("source rows mutated:\n got  %#v\n want %#v", rows, wantRows)
	}
}

func TestBuildVisualGridFallsBackWithoutLoss(t *testing.T) {
	rows := []ir.Row{
		{Y: 0, Runs: []ir.TextRun{{Text: "ab", StartCol: 0, EndCol: 3}}},
		{Y: 1, Runs: []ir.TextRun{{Text: "a", StartCol: 0, EndCol: 1}, {Text: "b", StartCol: 0, EndCol: 1}}},
		{Y: 2, Runs: []ir.TextRun{{Text: "a", StartCol: -1, EndCol: 0}}},
		{Y: 3, Runs: []ir.TextRun{{Text: "a", StartCol: 4, EndCol: 5}}},
	}
	want := cloneRows(rows)

	grid := buildVisualGrid(4, 4, rows, testCatalog())
	for y := range rows {
		if grid.rows[y].supported {
			t.Fatalf("row %d unexpectedly supported", y)
		}
		if !reflect.DeepEqual(grid.rows[y].fallback, rows[y]) {
			t.Fatalf("row %d fallback = %#v, want %#v", y, grid.rows[y].fallback, rows[y])
		}
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("source rows mutated:\n got  %#v\n want %#v", rows, want)
	}
}

func TestVisualGridBlankVisibilityMatchesStaticCells(t *testing.T) {
	bg := termcolor.ID(1)
	rows := []ir.Row{{Y: 0, Runs: []ir.TextRun{
		{Text: " ", StartCol: 0, EndCol: 1},
		{Text: " ", StartCol: 1, EndCol: 2, Attrs: ir.CellAttrs{BG: bg}},
		{Text: " ", StartCol: 2, EndCol: 3, Attrs: ir.CellAttrs{Underline: true}},
	}}}

	row := buildVisualGrid(3, 1, rows, testCatalog()).rows[0]
	if row.cells[0].visible || !row.cells[1].visible || !row.cells[2].visible {
		t.Fatalf("blank visibility = %#v", row.cells)
	}
	empty := buildVisualGrid(3, 1, nil, testCatalog()).rows[0]
	if !visualCellsEqual(&row, 0, &empty, 0) {
		t.Fatal("invisible default-background space did not equal an empty cell")
	}
	if visualCellsEqual(&row, 1, &empty, 1) || visualCellsEqual(&row, 2, &empty, 2) {
		t.Fatal("visible blank equaled an empty cell")
	}
}

func glyphs(row *visualRow) []visualGlyph {
	got := append([]visualGlyph(nil), row.glyphs...)
	for i := range got {
		got[i].attrs = ir.CellAttrs{}
	}
	return got
}

func cloneRows(rows []ir.Row) []ir.Row {
	clone := make([]ir.Row, len(rows))
	for i, row := range rows {
		clone[i] = row
		clone[i].Runs = append([]ir.TextRun(nil), row.Runs...)
	}
	return clone
}
