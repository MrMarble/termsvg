package svg

import (
	"context"
	stdcolor "image/color"
	"math/rand"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

type semanticCell struct {
	glyph        string
	span         int
	attrs        ir.CellAttrs
	continuation bool
	visible      bool
}

type semanticScreen [][]semanticCell

var parityOptions = []struct {
	name    string
	options []Option
}{
	{name: "frames-css-translate"},
	{name: "frames-smil-translate", options: []Option{WithAnimation(AnimationSMIL)}},
	{name: "frames-smil-href", options: []Option{WithAnimation(AnimationSMIL), WithFrameSwitch(FrameSwitchHref)}},
	{name: "bands-css-translate", options: []Option{WithLayout(LayoutBands)}},
	{name: "bands-smil-translate", options: []Option{WithLayout(LayoutBands), WithAnimation(AnimationSMIL)}},
	{name: "bands-smil-href", options: []Option{
		WithLayout(LayoutBands), WithAnimation(AnimationSMIL), WithFrameSwitch(FrameSwitchHref),
	}},
	{name: "regions-css-translate", options: []Option{WithLayout(LayoutRegions)}},
	{name: "regions-smil-translate", options: []Option{WithLayout(LayoutRegions), WithAnimation(AnimationSMIL)}},
	{name: "regions-smil-href", options: []Option{
		WithLayout(LayoutRegions), WithAnimation(AnimationSMIL), WithFrameSwitch(FrameSwitchHref),
	}},
}

func TestSemanticScreenPreservesMixedWidthRunAtomically(t *testing.T) {
	rec := parityRecording(3, 1, [][]ir.Row{{
		parityRow(0, ir.TextRun{Text: "界A", StartCol: 0, EndCol: 3}),
	}})

	screen := screenFromRows(rec, rec.Frames[0].Rows, 0, 0)
	if screen[0][0].glyph != "界A" || screen[0][0].span != 3 ||
		!screen[0][1].continuation || !screen[0][2].continuation {
		t.Fatalf("mixed-width cells = %#v", screen[0])
	}
}

func TestSemanticScreenPreservesCombiningAndVariationSequences(t *testing.T) {
	rec := parityRecording(3, 1, [][]ir.Row{{
		parityRow(0,
			ir.TextRun{Text: "e\u0301", StartCol: 0, EndCol: 1},
			ir.TextRun{Text: "✈️", StartCol: 1, EndCol: 3},
		),
	}})

	screen := screenFromRows(rec, rec.Frames[0].Rows, 0, 0)
	if screen[0][0].glyph != "e\u0301" || screen[0][0].span != 1 ||
		screen[0][1].glyph != "✈️" || screen[0][1].span != 2 || !screen[0][2].continuation {
		t.Fatalf("combined cells = %#v", screen[0])
	}
	ambiguous := semanticRunCells(rec, ir.TextRun{Text: "e\u0301\x00", StartCol: 0, EndCol: 3})
	if ambiguous[0].glyph != "e\u0301\x00" || ambiguous[0].span != 3 {
		t.Fatalf("combined run with NUL = %#v", ambiguous)
	}
}

func TestSemanticScreenUsesExplicitWideContinuationCells(t *testing.T) {
	rec := parityRecording(3, 1, [][]ir.Row{{
		parityRow(0, ir.TextRun{Text: "界\x00A", StartCol: 0, EndCol: 3}),
	}})

	screen := screenFromRows(rec, rec.Frames[0].Rows, 0, 0)
	if screen[0][0].glyph != "界" || !screen[0][1].continuation || screen[0][2].glyph != "A" {
		t.Fatalf("explicit wide cells = %#v", screen[0])
	}
}

func TestSemanticPlaybackParityTUIFixtures(t *testing.T) {
	for name, rec := range tuiParityFixtures() {
		t.Run(name, func(t *testing.T) {
			for _, variant := range parityOptions {
				t.Run(variant.name, func(t *testing.T) { assertSemanticParity(t, rec, variant.options...) })
			}
		})
	}
}

func TestAutoStylePreservesPreparedSemantics(t *testing.T) {
	for name, rec := range tuiParityFixtures() {
		for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions} {
			t.Run(name+"/"+string(layout), func(t *testing.T) {
				assertSemanticParity(t, rec, WithLayout(layout), WithStyleMode(StyleAuto))
			})
		}
	}
}

func TestSemanticPlaybackParityDeterministicRandomRecordings(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5445524d535647)) //nolint:gosec // deterministic test coverage
	for i := range 64 {
		rec := randomParityRecording(rng)
		for _, variant := range parityOptions {
			t.Run(strconv.Itoa(i)+"/"+variant.name, func(t *testing.T) {
				assertSemanticParity(t, rec, variant.options...)
			})
		}
	}
}

func TestRegionSemanticParityLoopAndContentStateEdges(t *testing.T) {
	for _, loop := range []struct {
		name  string
		count int
	}{{name: "infinite", count: 0}, {name: "finite", count: 2}} {
		for _, content := range []struct {
			name string
			rows []ir.Row
		}{
			{name: "zero-content", rows: nil},
			{name: "one-content", rows: []ir.Row{parityRow(0, parityRun("x", 0, ir.CellAttrs{}))}},
		} {
			t.Run(loop.name+"/"+content.name, func(t *testing.T) {
				rec := parityRecording(2, 1, [][]ir.Row{content.rows})
				config := renderer.DefaultConfig()
				config.LoopCount = loop.count
				assertSemanticParityWithConfig(t, rec, config, WithLayout(LayoutRegions))
			})
		}
	}
}

func TestEffectiveTimesSamplesTransitionBoundaries(t *testing.T) {
	rec := parityRecording(1, 1, [][]ir.Row{
		{parityRow(0, parityRun("a", 0, ir.CellAttrs{}))},
		{parityRow(0, parityRun("b", 0, ir.CellAttrs{}))},
	})
	want := []time.Duration{0, 1, time.Second - 1, time.Second, time.Second + 1, 2 * time.Second}
	if got := effectiveTimes(rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("effective times = %v; want %v", got, want)
	}
}

func TestRegionSemanticParityWithBoundedAndUnlimitedOptimization(t *testing.T) {
	rec := tuiParityFixtures()["scrolling-table"]
	for _, test := range []struct {
		name   string
		budget int
	}{{name: "bounded", budget: regionCandidateEvaluationBudget}, {name: "unlimited", budget: 0}} {
		t.Run(test.name, func(t *testing.T) {
			config := renderer.DefaultConfig()
			options := DefaultOptions()
			options.Layout = LayoutRegions
			plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, 0, config.LoopCount)
			if err != nil {
				t.Fatal(err)
			}
			c := canvas{
				rec: rec, plan: plan, config: *config, options: options,
				classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{},
			}
			regions, err := c.optimizeDynamicRegionsWithBudget(
				context.Background(), buildDynamicRegions(&plan, rec.Colors), test.budget,
			)
			if err != nil {
				t.Fatal(err)
			}
			assertNoRegionPaintOverlap(t, rec, regions)
			prepared, err := c.prepareLocalViewports(context.Background(), c.regionBands(regions))
			if err != nil {
				t.Fatal(err)
			}
			for _, at := range effectiveTimes(rec) {
				want := screenFromRows(rec, sourceRowsAt(rec, at), 0, 0)
				if got := preparedScreenAt(rec, &plan, &prepared, &options, at); !reflect.DeepEqual(got, want) {
					t.Fatalf("prepared screen at %v differs\n got: %#v\nwant: %#v", at, got, want)
				}
			}
		})
	}
}

func assertSemanticParity(t *testing.T, rec *ir.Recording, opts ...Option) {
	t.Helper()
	assertSemanticParityWithConfig(t, rec, renderer.DefaultConfig(), opts...)
}

func assertSemanticParityWithConfig(t *testing.T, rec *ir.Recording, config *renderer.Config, opts ...Option) {
	t.Helper()
	options := DefaultOptions()
	for _, option := range opts {
		option(&options)
	}
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, options.MaxFPS, config.LoopCount)
	if err != nil {
		t.Fatalf("build semantic plan: %v", err)
	}
	assertPlanParity(t, rec, config, &options, &plan)
}

// assertLayeredSemanticParity checks the plan after interval-static layer
// selection, which the renderer applies on top of buildSemanticPlan.
func assertLayeredSemanticParity(t *testing.T, rec *ir.Recording, opts ...Option) renderPlan {
	t.Helper()
	config := renderer.DefaultConfig()
	options := DefaultOptions()
	for _, option := range opts {
		option(&options)
	}
	plan, err := layeredPlan(context.Background(), rec, config, &options)
	if err != nil {
		t.Fatalf("layered plan: %v", err)
	}
	assertPlanParity(t, rec, config, &options, &plan)
	return plan
}

func assertPlanParity(t *testing.T, rec *ir.Recording, config *renderer.Config, options *Options, plan *renderPlan) {
	t.Helper()
	before := cloneRecording(rec)
	c := canvas{
		rec: rec, plan: *plan, config: *config, options: *options,
		classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{},
	}
	if options.Layout == LayoutRegions {
		regions := buildDynamicRegions(plan, rec.Colors)
		optimized, err := c.optimizeDynamicRegions(context.Background(), regions)
		if err != nil {
			t.Fatalf("optimize regions: %v", err)
		}
		assertNoRegionPaintOverlap(t, rec, optimized)
	}
	prepared, err := c.prepareContentContext(context.Background())
	if err != nil {
		t.Fatalf("prepare content: %v", err)
	}

	for _, at := range effectiveTimes(rec) {
		want := screenFromRows(rec, sourceRowsAt(rec, at), 0, 0)
		planned := append(slices.Clone(plan.staticRows), plan.layerRowsAt(at)...)
		planned = append(planned, timelineStateAt(plan.content, at)...)
		if got := screenFromRows(rec, planned, 0, 0); !reflect.DeepEqual(got, want) {
			t.Fatalf("planned screen at %v differs\n got: %#v\nwant: %#v", at, got, want)
		}
		if got := preparedScreenAt(rec, plan, &prepared, options, at); !reflect.DeepEqual(got, want) {
			t.Fatalf("prepared screen at %v differs\n got: %#v\nwant: %#v", at, got, want)
		}
		gotCursor := visibleCursor(timelineStateAt(plan.cursor, at))
		if expected := visibleCursor(sourceCursorAt(rec, at)); gotCursor != expected {
			t.Fatalf("cursor at %v = %#v, want %#v", at, gotCursor, expected)
		}
	}
	if !reflect.DeepEqual(rec, before) {
		t.Fatal("semantic planning mutated source IR")
	}
}

func preparedScreenAt(
	rec *ir.Recording,
	plan *renderPlan,
	content *preparedContent,
	options *Options,
	at time.Duration,
) semanticScreen {
	rows := append(slices.Clone(plan.staticRows), plan.layerRowsAt(at)...)
	if options.Layout != LayoutBands && options.Layout != LayoutRegions && options.Layout != LayoutScroll {
		_, states := contentKeyframesFor(plan.content)
		state := rowsStateIndex(states, timelineStateAt(plan.content, at))
		if state >= 0 && state < len(content.frameRows) {
			rows = append(rows, renderedRows(content.frameRows[state], 0, 0)...)
		}
		return screenFromRows(rec, rows, 0, 0)
	}
	for bandIndex := range content.bands {
		band := &content.bands[bandIndex]
		if band.kind == bandScrollTape {
			_, states := contentKeyframesFor(band.content)
			state := rowsStateIndex(states, timelineStateAt(band.content, at))
			offset := 0
			for i, frame := range band.keyframes {
				if frame.state == state {
					offset = band.offsets[i].state
					break
				}
			}
			for _, row := range renderedRows(band.tapeRows, band.x, band.y-offset) {
				if row.Y >= band.y && row.Y < band.y+band.height {
					rows = append(rows, row)
				}
			}
			continue
		}
		_, states := contentKeyframesFor(band.content)
		state := rowsStateIndex(states, timelineStateAt(band.content, at))
		if state >= 0 && state < len(band.rows) {
			rows = append(rows, renderedRows(band.rows[state], band.x, band.y)...)
		}
	}
	return screenFromRows(rec, rows, 0, 0)
}

func assertNoRegionPaintOverlap(t *testing.T, rec *ir.Recording, regions []dynamicRegion) {
	t.Helper()
	for _, at := range effectiveTimes(rec) {
		painted := make(map[[2]int]int)
		for index, region := range regions {
			for _, row := range timelineStateAt(region.content, at) {
				for _, run := range row.Runs {
					for offset, cell := range semanticRunCells(rec, run) {
						if !cell.visible {
							continue
						}
						position := [2]int{region.x + run.StartCol + offset, region.y + row.Y}
						if owner, ok := painted[position]; ok {
							t.Fatalf("regions %d and %d both paint cell %v at %v", owner, index, position, at)
						}
						painted[position] = index
					}
				}
			}
		}
	}
}

func rowsStateIndex(states [][]ir.Row, target []ir.Row) int {
	for i, state := range states {
		if rowsEqual(state, target) {
			return i
		}
	}
	return -1
}

// renderedRows reconstructs the screen rows a state serializes, expanding
// block references and definitions to the rows they cover.
func renderedRows(rows []*renderedRow, x, y int) []ir.Row {
	out := make([]ir.Row, 0, len(rows))
	for _, rendered := range rows {
		covered := rendered.rows
		if len(covered) == 0 {
			covered = []ir.Row{rendered.row}
		}
		for _, row := range covered {
			row.Y += y
			row.Runs = slices.Clone(row.Runs)
			for j := range row.Runs {
				row.Runs[j].StartCol += x
				row.Runs[j].EndCol += x
			}
			out = append(out, row)
		}
	}
	return out
}

func timelineStateAt[T any](timeline timeline[T], at time.Duration) T {
	var state T
	for _, point := range timeline.points {
		if point.time > at {
			break
		}
		state = point.state
	}
	return state
}

func effectiveTimes(rec *ir.Recording) []time.Duration {
	seen := map[time.Duration]bool{0: true, rec.Duration: true}
	for _, frame := range rec.Frames {
		seen[frame.Time] = true
		if frame.Time > 0 {
			seen[frame.Time-1] = true
		}
		if frame.Time < rec.Duration {
			seen[frame.Time+1] = true
		}
	}
	times := make([]time.Duration, 0, len(seen))
	for at := range seen {
		times = append(times, at)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	return times
}

func sourceRowsAt(rec *ir.Recording, at time.Duration) []ir.Row {
	var rows []ir.Row
	for _, frame := range rec.Frames {
		if frame.Time > at {
			break
		}
		rows = frame.Rows
	}
	return rows
}

func sourceCursorAt(rec *ir.Recording, at time.Duration) ir.Cursor {
	var cursor ir.Cursor
	for _, frame := range rec.Frames {
		if frame.Time > at {
			break
		}
		cursor = frame.Cursor
	}
	return cursor
}

func visibleCursor(cursor ir.Cursor) ir.Cursor {
	if !cursor.Visible {
		return ir.Cursor{}
	}
	return cursor
}

func screenFromRows(rec *ir.Recording, rows []ir.Row, xOffset, yOffset int) semanticScreen {
	screen := make(semanticScreen, rec.Height)
	for y := range screen {
		screen[y] = make([]semanticCell, rec.Width)
	}
	for _, row := range rows {
		y := row.Y + yOffset
		if y < 0 || y >= rec.Height {
			continue
		}
		for _, run := range row.Runs {
			for i, cell := range semanticRunCells(rec, run) {
				col := run.StartCol + xOffset + i
				if col < 0 || col >= rec.Width {
					continue
				}
				if cell.visible {
					screen[y][col] = cell
				}
			}
		}
	}
	return screen
}

func semanticRunCells(rec *ir.Recording, run ir.TextRun) []semanticCell {
	width := runEndCol(run) - run.StartCol
	if width <= 0 {
		return nil
	}
	runes := []rune(run.Text)
	if explicitCellRunes(runes, width) {
		cells := make([]semanticCell, width)
		for i, char := range runes {
			cells[i] = semanticCell{glyph: string(char), span: 1, attrs: run.Attrs}
			if char == 0 {
				cells[i].glyph = ""
				cells[i].continuation = true
			}
			cells[i].visible = semanticCellVisible(rec, cells[i])
		}
		return cells
	}

	lead := semanticCell{glyph: run.Text, span: width, attrs: run.Attrs}
	lead.visible = semanticCellVisible(rec, lead)
	cells := make([]semanticCell, width)
	cells[0] = lead
	for i := 1; i < width; i++ {
		cells[i] = semanticCell{attrs: run.Attrs, continuation: true, visible: lead.visible}
	}
	return cells
}

func explicitCellRunes(runes []rune, width int) bool {
	if len(runes) != width {
		return false
	}
	for i, char := range runes {
		if char <= 0x7f {
			continue
		}
		if unicode.IsMark(char) || i+1 == len(runes) || runes[i+1] != 0 {
			return false
		}
	}
	return true
}

func semanticCellVisible(rec *ir.Recording, cell semanticCell) bool {
	return cell.continuation || strings.TrimSpace(cell.glyph) != "" ||
		cell.attrs.Underline || !rec.Colors.IsDefault(cell.attrs.BG)
}

func tuiParityFixtures() map[string]*ir.Recording {
	return map[string]*ir.Recording{
		"static-dashboard-counter": parityRecording(12, 3, [][]ir.Row{
			{parityRow(0, parityRun("CPU: 1", 0, ir.CellAttrs{})), parityRow(2, parityRun("ready", 0, ir.CellAttrs{}))},
			{parityRow(0, parityRun("CPU: 2", 0, ir.CellAttrs{})), parityRow(2, parityRun("ready", 0, ir.CellAttrs{}))},
		}),
		"distant-same-row-counters": parityRecording(14, 2, [][]ir.Row{
			{parityRow(0, parityRun("1", 0, ir.CellAttrs{}), parityRun("9", 12, ir.CellAttrs{}))},
			{parityRow(0, parityRun("2", 0, ir.CellAttrs{}), parityRun("8", 12, ir.CellAttrs{}))},
		}),
		"adjacent-progress-bars": parityRecording(10, 3, [][]ir.Row{
			{parityRow(0, parityRun("[##  ]", 0, ir.CellAttrs{})), parityRow(1, parityRun("[#   ]", 0, ir.CellAttrs{}))},
			{parityRow(0, parityRun("[### ]", 0, ir.CellAttrs{})), parityRow(1, parityRun("[##  ]", 0, ir.CellAttrs{}))},
		}),
		"scrolling-table": parityRecording(8, 3, [][]ir.Row{
			{parityRow(0, parityRun("one", 0, ir.CellAttrs{})), parityRow(1, parityRun("two", 0, ir.CellAttrs{}))},
			{parityRow(0, parityRun("two", 0, ir.CellAttrs{})), parityRow(1, parityRun("three", 0, ir.CellAttrs{}))},
		}),
		"full-screen-redraw": parityRecording(6, 2, [][]ir.Row{
			{parityRow(0, parityRun("aaaa", 0, ir.CellAttrs{})), parityRow(1, parityRun("bbbb", 0, ir.CellAttrs{}))},
			{
				parityRow(0, parityRun("xxxx", 0, ir.CellAttrs{Bold: true})),
				parityRow(1, parityRun("yyyy", 0, ir.CellAttrs{Italic: true})),
			},
		}),
		"help-overlay": parityRecording(10, 3, [][]ir.Row{
			{parityRow(0, parityRun("main", 0, ir.CellAttrs{}))},
			{
				parityRow(0, parityRun("main", 0, ir.CellAttrs{})),
				parityRow(1, parityRun("? help", 2, ir.CellAttrs{Underline: true})),
			},
			{parityRow(0, parityRun("main", 0, ir.CellAttrs{}))},
		}),
		"colored-blanks": coloredBlankRecording(),
		"wide-glyph-fallback": parityRecording(5, 1, [][]ir.Row{
			{parityRow(0, ir.TextRun{Text: "界A", StartCol: 0, EndCol: 3})},
			{parityRow(0,
				ir.TextRun{Text: "e\u0301", StartCol: 0, EndCol: 1},
				ir.TextRun{Text: "✈️", StartCol: 1, EndCol: 3},
			)},
			{parityRow(0, ir.TextRun{Text: "界\x00A", StartCol: 0, EndCol: 3})},
		}),
	}
}

func parityRecording(width, height int, states [][]ir.Row) *ir.Recording {
	rec := experimentalRecording()
	rec.Width, rec.Height = width, height
	rec.Frames = make([]ir.Frame, len(states))
	for i, rows := range states {
		rec.Frames[i] = ir.Frame{
			Time: time.Duration(i) * time.Second, Rows: rows,
			Cursor: ir.Cursor{Col: i % width, Row: i % height, Visible: i%2 == 0},
		}
	}
	rec.Duration = time.Duration(max(1, len(states))) * time.Second
	return rec
}

func coloredBlankRecording() *ir.Recording {
	rec := parityRecording(5, 1, nil)
	palette := termcolor.Standard()
	bg := rec.Colors.Register(termcolor.FromRGB(30, 40, 50), &palette)
	rec.Frames = []ir.Frame{
		{Rows: []ir.Row{parityRow(0, parityRun("  ", 1, ir.CellAttrs{BG: bg}))}},
		{Time: time.Second, Rows: []ir.Row{parityRow(0, parityRun("  ", 2, ir.CellAttrs{BG: bg, Underline: true}))}},
	}
	rec.Duration = 2 * time.Second
	return rec
}

func parityRow(y int, runs ...ir.TextRun) ir.Row { return ir.Row{Y: y, Runs: runs} }

func parityRun(text string, start int, attrs ir.CellAttrs) ir.TextRun {
	return ir.TextRun{Text: text, StartCol: start, EndCol: start + len([]rune(text)), Attrs: attrs}
}

func randomParityRecording(rng *rand.Rand) *ir.Recording {
	width, height := rng.Intn(8)+1, rng.Intn(5)+1
	catalog := termcolor.NewCatalog(stdcolor.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}, stdcolor.RGBA{A: 0xff})
	palette := termcolor.Standard()
	fg := catalog.Register(termcolor.FromRGB(200, 100, 50), &palette)
	bg := catalog.Register(termcolor.FromRGB(20, 60, 100), &palette)
	frameCount := rng.Intn(7) + 2
	frames := make([]ir.Frame, frameCount)
	var previous []ir.Row
	var at time.Duration
	for i := range frames {
		if i > 0 && i%4 != 0 {
			at += time.Duration(rng.Intn(3)) * 20 * time.Millisecond
		}
		rows := randomRows(rng, width, height, fg, bg)
		if i > 0 && i%5 == 0 {
			rows = previous
		}
		frames[i] = ir.Frame{Time: at, Index: i, Rows: rows, Cursor: ir.Cursor{
			Col: rng.Intn(width), Row: rng.Intn(height), Visible: rng.Intn(3) != 0,
		}}
		previous = rows
	}
	return &ir.Recording{Width: width, Height: height, Duration: at + time.Second, Frames: frames, Colors: catalog}
}

func randomRows(rng *rand.Rand, width, height int, fg, bg termcolor.ID) []ir.Row {
	rows := make([]ir.Row, 0, height)
	for y := range height {
		if rng.Intn(4) == 0 {
			continue
		}
		row := ir.Row{Y: y}
		for col := 0; col < width; {
			if rng.Intn(3) == 0 {
				col++
				continue
			}
			attrs := ir.CellAttrs{
				Bold: rng.Intn(5) == 0, Italic: rng.Intn(6) == 0,
				Underline: rng.Intn(5) == 0, Dim: rng.Intn(6) == 0,
			}
			if rng.Intn(3) == 0 {
				attrs.FG = fg
			}
			if rng.Intn(4) == 0 {
				attrs.BG = bg
			}
			char := []rune("abcd")[rng.Intn(4)]
			if rng.Intn(4) == 0 {
				char = ' '
			}
			row.Runs = append(row.Runs, parityRun(string(char), col, attrs))
			col++
		}
		if len(row.Runs) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

func cloneRecording(rec *ir.Recording) *ir.Recording {
	clone := *rec
	clone.Frames = slices.Clone(rec.Frames)
	for i, frame := range rec.Frames {
		clone.Frames[i].Rows = slices.Clone(frame.Rows)
		for j, row := range frame.Rows {
			clone.Frames[i].Rows[j] = row
			clone.Frames[i].Rows[j].Runs = slices.Clone(row.Runs)
		}
	}
	return &clone
}
