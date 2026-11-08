package svg

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// dashboardRecording starts and ends with a blank screen. In between, labels
// and an axis stay put while a counter changes on every state, so the labels
// are constant over states 1..n-2 but never over the whole recording.
func dashboardRecording(states int) *ir.Recording {
	rows := make([][]ir.Row, states)
	for i := 1; i < states-1; i++ {
		// Every value is distinct so no two states fold into one definition.
		value := strings.Repeat("#", i%7+1) + strconv.Itoa(i)
		rows[i] = []ir.Row{
			parityRow(0, parityRun("load: "+value+strings.Repeat(" ", 20-len(value))+"<- press q to quit", 0, ir.CellAttrs{})),
			parityRow(1, parityRun("memory used: "+value, 0, ir.CellAttrs{})),
			parityRow(3, parityRun("axis: |....+....|....+....|....+....|", 0, ir.CellAttrs{})),
		}
	}
	rec := parityRecording(60, 4, rows)
	for i := range rec.Frames {
		rec.Frames[i].Cursor = ir.Cursor{}
	}
	return rec
}

func TestIntervalLayersHoistCellsConstantOverStateRuns(t *testing.T) {
	rec := dashboardRecording(40)
	for _, variant := range []struct {
		name    string
		options []Option
		want    []string
	}{
		{name: "smil", options: []Option{WithAnimation(AnimationSMIL)}, want: []string{
			`<g visibility="hidden"><animate attributeName="visibility" values="hidden;visible;hidden;hidden"` +
				` keyTimes="0;.025;.975;1"`,
		}},
		{name: "css", want: []string{
			`@keyframes l0{0%{visibility:hidden}2.5%{visibility:visible}97.5%{visibility:hidden}100%{visibility:hidden}}`,
			`<g style="animation:l0 40s infinite step-end" visibility="hidden">`,
		}},
	} {
		t.Run(variant.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := New(renderer.DefaultConfig(), variant.options...).Render(context.Background(), rec, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			svg := buf.String()
			for _, want := range variant.want {
				if !strings.Contains(svg, want) {
					t.Fatalf("missing %q in:\n%s", want, svg)
				}
			}
			for _, once := range []string{"press q to quit", "axis: |....+....|", ">load:", ">memory used:"} {
				if got := strings.Count(svg, once); got != 1 {
					t.Fatalf("%q serialized %d times, want once:\n%s", once, got, svg)
				}
			}
			// The first "#" of every value is constant as well and joins the
			// layer; the changing remainder stays in the states at its own column.
			if strings.Count(svg, "load: ") != 1 || !strings.Contains(svg, `x="84" y="20">7</text>`) {
				t.Fatalf("counter values are not left in the states at their column:\n%s", svg)
			}
		})
	}
}

func TestIntervalLayersPreserveSemanticsAcrossLayouts(t *testing.T) {
	// Block sharing already folds every state of this dashboard into one
	// reference, so the layer needs enough states to pay for itself.
	rec := dashboardRecording(40)
	for _, variant := range parityOptions {
		t.Run(variant.name, func(t *testing.T) {
			plan := assertLayeredSemanticParity(t, rec, variant.options...)
			if len(plan.layers) == 0 {
				t.Fatalf("no interval layer selected for %s", variant.name)
			}
		})
	}
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions, LayoutScroll} {
		t.Run(string(layout)+"/auto-style", func(t *testing.T) {
			assertLayeredSemanticParity(t, rec, WithLayout(layout), WithStyleMode(StyleAuto))
		})
	}
}

func TestIntervalLayersMetricsMatchSerializedStructure(t *testing.T) {
	rec := dashboardRecording(40)
	for _, variant := range parityOptions {
		t.Run(variant.name, func(t *testing.T) {
			config := renderer.DefaultConfig()
			options := DefaultOptions()
			for _, apply := range variant.options {
				apply(&options)
			}
			plan, err := layeredPlan(context.Background(), rec, config, &options)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.layers) == 0 {
				t.Fatal("no interval layer selected")
			}
			candidate, err := prepareCandidate(context.Background(), rec, &plan, config, &options)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			parsed := CandidateMetrics{}
			writer := &candidateWriter{w: &out, metrics: &parsed}
			r := New(config, variant.options...)
			if err := r.serializeCandidate(context.Background(), rec, writer, candidate); err != nil {
				t.Fatal(err)
			}
			writer.finish()
			got := candidate.metrics
			if candidate.cost.finalBytes != parsed.FinalBytes {
				t.Fatalf("prepared bytes = %d, serialized %d", candidate.cost.finalBytes, parsed.FinalBytes)
			}
			if got.XMLNodes != parsed.XMLNodes || got.DefinitionNodes != parsed.DefinitionNodes ||
				got.ActiveNodes != parsed.ActiveNodes || got.TextNodes != parsed.TextNodes ||
				got.RectNodes != parsed.RectNodes || got.GroupNodes != parsed.GroupNodes ||
				got.UseNodes != parsed.UseNodes || got.AnimationNodes != parsed.AnimationNodes ||
				got.AnimatedElements != parsed.AnimatedElements {
				t.Fatalf("prepared metrics = %#v; serialized = %#v", got, parsed)
			}
		})
	}
}

func TestIntervalLayersKeepUnprofitableCellsInStates(t *testing.T) {
	// One cell constant over three short states cannot pay for an animation.
	rec := parityRecording(10, 2, [][]ir.Row{
		nil,
		{parityRow(0, parityRun("a1", 0, ir.CellAttrs{}))},
		{parityRow(0, parityRun("a2", 0, ir.CellAttrs{}))},
		{parityRow(0, parityRun("a3", 0, ir.CellAttrs{}))},
		nil,
	})
	options := DefaultOptions()
	plan, err := layeredPlan(context.Background(), rec, renderer.DefaultConfig(), &options)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.layers) != 0 {
		t.Fatalf("unprofitable layer selected: %#v", plan.layers)
	}
	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "@keyframes l") || strings.Contains(buf.String(), "animation:l") {
		t.Fatalf("unexpected layer markup:\n%s", buf.String())
	}
}

func TestIntervalLayersVisibleFromStartHideAtEnd(t *testing.T) {
	states := make([][]ir.Row, 12)
	for i := range 11 {
		bar := strings.Repeat("=", i+1)
		states[i] = []ir.Row{parityRow(0, parityRun(
			"status: "+bar+strings.Repeat(" ", 22-len(bar))+"press q to quit, h for help", 0, ir.CellAttrs{}))}
	}
	rec := parityRecording(60, 1, states)
	var buf bytes.Buffer
	r := New(renderer.DefaultConfig(), WithAnimation(AnimationSMIL))
	if err := r.Render(context.Background(), rec, &buf); err != nil {
		t.Fatal(err)
	}
	svg := buf.String()
	const want = `<g><animate attributeName="visibility" values="visible;hidden;hidden" keyTimes="0;.91667;1"`
	if !strings.Contains(svg, want) {
		t.Fatalf("layer visible from the start was not encoded without a hidden base value:\n%s", svg)
	}
	if strings.Count(svg, "status:") != 1 || strings.Count(svg, "press q to quit") != 1 {
		t.Fatalf("labels serialized more than once:\n%s", svg)
	}
	assertLayeredSemanticParity(t, rec, WithAnimation(AnimationSMIL))
}

func TestCellsToRowBridgesShortInertGaps(t *testing.T) {
	rec := parityRecording(40, 1, nil)
	palette := termcolor.Standard()
	bg := rec.Colors.Register(termcolor.FromRGB(30, 40, 50), &palette)
	build := func(text string, attrs ir.CellAttrs) ([]terminalCell, []bool) {
		cells := make([]terminalCell, 40)
		include := make([]bool, 40)
		for i := range cells {
			cells[i] = terminalCell{char: ' ', attrs: attrs}
		}
		for i, char := range text {
			cells[i] = terminalCell{char: char, attrs: attrs}
			include[i] = cellVisible(cells[i], rec.Colors)
		}
		return cells, include
	}
	cells, include := build("ab   cd", ir.CellAttrs{})
	row := cellsToRow(0, cells, include, rec.Colors)
	if len(row.Runs) != 1 || row.Runs[0].Text != "ab   cd" || row.Runs[0].EndCol != 7 {
		t.Fatalf("short gap was not bridged: %#v", row.Runs)
	}
	cells, include = build("ab"+strings.Repeat(" ", maxInertGap)+"cd", ir.CellAttrs{})
	if row := cellsToRow(0, cells, include, rec.Colors); len(row.Runs) != 2 {
		t.Fatalf("long gap was bridged: %#v", row.Runs)
	}
	cells, include = build("ab   cd", ir.CellAttrs{BG: bg})
	if row := cellsToRow(0, cells, include, rec.Colors); len(row.Runs) != 1 || row.Runs[0].Text != "ab   cd" {
		t.Fatalf("visible background spaces are part of the run: %#v", row.Runs)
	}
	cells, include = build("ab   cd", ir.CellAttrs{Underline: true})
	if row := cellsToRow(0, cells, include, rec.Colors); len(row.Runs) != 1 {
		t.Fatalf("underlined spaces are visible cells and stay in the run: %#v", row.Runs)
	}
	cells, include = build("ab   cd", ir.CellAttrs{})
	cells[4].attrs = ir.CellAttrs{Bold: true}
	if row := cellsToRow(0, cells, include, rec.Colors); len(row.Runs) != 1 || row.Runs[0].Text != "ab   cd" {
		t.Fatalf("gap attributes must not matter: %#v", row.Runs)
	}
	cells, include = build("ab   cd", ir.CellAttrs{})
	cells[5].attrs = ir.CellAttrs{Bold: true}
	cells[6].attrs = ir.CellAttrs{Bold: true}
	if row := cellsToRow(0, cells, include, rec.Colors); len(row.Runs) != 2 {
		t.Fatalf("differently styled cells must not be bridged: %#v", row.Runs)
	}
}

func TestLayerVisibilityKeyframes(t *testing.T) {
	c := canvas{plan: renderPlan{duration: 10 * time.Second}}
	frames := c.layerVisibility(intervalLayer{from: time.Second, to: 9 * time.Second})
	if len(frames) != 4 || frames[0].state || !frames[1].state || frames[2].state ||
		frames[1].selector != "10%" || frames[2].selector != "90%" {
		t.Fatalf("bounded layer keyframes = %#v", frames)
	}
	frames = c.layerVisibility(intervalLayer{from: time.Second, to: 10 * time.Second, open: true})
	if len(frames) != 3 || frames[0].state || !frames[1].state || !frames[2].state || frames[2].selector != "100%" {
		t.Fatalf("open layer keyframes = %#v", frames)
	}
	// A state that starts exactly at the duration still hides the layer there.
	frames = c.layerVisibility(intervalLayer{from: time.Second, to: 10 * time.Second})
	if len(frames) != 3 || frames[2].state || frames[2].selector != "100%" {
		t.Fatalf("layer ending at the duration keyframes = %#v", frames)
	}
	open := intervalLayer{from: time.Second, to: 10 * time.Second, open: true, rows: []ir.Row{{Y: 1}}}
	closed := intervalLayer{from: time.Second, to: 10 * time.Second, rows: []ir.Row{{Y: 2}}}
	plan := renderPlan{duration: 10 * time.Second, layers: []intervalLayer{open, closed}}
	if rows := plan.layerRowsAt(10 * time.Second); len(rows) != 1 || rows[0].Y != 1 {
		t.Fatalf("only the open layer is visible at the end: %#v", rows)
	}
	if rows := plan.layerRowsAt(5 * time.Second); len(rows) != 2 {
		t.Fatalf("both layers are visible in the middle: %#v", rows)
	}
}
