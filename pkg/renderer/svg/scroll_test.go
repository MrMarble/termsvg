//nolint:lll // Compact fixture construction keeps scroll state transitions readable.
package svg

import (
	"bytes"
	"context"
	stdcolor "image/color"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestScrollLayoutBuildsProfitableStrictUpwardTape(t *testing.T) {
	rec := scrollRecording(120, 40, 21)
	before := cloneRecording(rec)
	config := renderer.DefaultConfig()
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	c := scrollCanvas(rec, &plan, config)
	content, err := c.prepareContentContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(content.bands) != 1 || content.bands[0].kind != bandScrollTape {
		t.Fatalf("prepared bands = %#v, want one scroll tape", content.bands)
	}
	band := content.bands[0]
	if len(band.tapeRows) != 60 || len(band.offsets) != 21 || band.offsets[0].state != 0 || band.offsets[len(band.offsets)-1].state != 20 {
		t.Fatalf("tape rows/keyframes = %d/%#v", len(band.tapeRows), band.offsets)
	}
	if !reflect.DeepEqual(rec, before) {
		t.Fatal("scroll preparation mutated source IR")
	}
}

func TestScrollLayoutSupportsTwoRowShifts(t *testing.T) {
	rec := scrollRecording(120, 40, 5)
	rec.Frames = []ir.Frame{rec.Frames[0], rec.Frames[2], rec.Frames[4]}
	config := renderer.DefaultConfig()
	plan, err := buildSemanticPlan(context.Background(), rec, false, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := scrollCanvas(rec, &plan, config)
	content, err := c.prepareContentContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(content.bands) != 1 || content.bands[0].kind != bandScrollTape || content.bands[0].offsets[1].state != 2 {
		t.Fatalf("two-row shift did not form tape: %#v", content.bands)
	}
}

func TestScrollLayoutRejectsNonScrollTransitions(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ir.Recording)
	}{
		{name: "downward", edit: reverseScrollFrames},
		{name: "direction reversal", edit: func(rec *ir.Recording) { rec.Frames[2].Rows = cloneScrollRows(rec.Frames[0].Rows) }},
		{name: "overlap edit", edit: func(rec *ir.Recording) { rec.Frames[1].Rows[5].Runs[0].Text = "edited" }},
		{name: "unsupported row", edit: func(rec *ir.Recording) {
			rec.Frames[0].Rows[3].Runs[0] = ir.TextRun{Text: "éx", StartCol: 0, EndCol: 2}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := scrollRecording(20, 8, 3)
			test.edit(rec)
			config := renderer.DefaultConfig()
			plan, err := buildSemanticPlan(context.Background(), rec, false, 0, 1)
			if err != nil {
				t.Fatal(err)
			}
			c := scrollCanvas(rec, &plan, config)
			content, err := c.prepareContentContext(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, band := range content.bands {
				if band.kind == bandScrollTape {
					t.Fatalf("strict detector accepted %s", test.name)
				}
			}
			assertSemanticParity(t, rec, WithLayout(LayoutScroll))
		})
	}
}

//nolint:gocognit // each output check applies to one or both animation backends
func TestScrollTapeCSSAndSMILEmissionRemainLossless(t *testing.T) {
	rec := scrollRecording(120, 40, 21)
	config := renderer.DefaultConfig()
	config.LoopCount = 2
	for _, animation := range []AnimationMode{AnimationCSS, AnimationSMIL} {
		t.Run(string(animation), func(t *testing.T) {
			frameSwitch := FrameSwitchTranslate
			if animation == AnimationSMIL {
				frameSwitch = FrameSwitchHref
			}
			assertSemanticParityWithConfig(t, rec, config, WithLayout(LayoutScroll), WithAnimation(animation), WithFrameSwitch(frameSwitch))
			var out bytes.Buffer
			if err := New(config, WithLayout(LayoutScroll), WithAnimation(animation), WithFrameSwitch(frameSwitch)).Render(context.Background(), rec, &out); err != nil {
				t.Fatal(err)
			}
			got := out.String()
			if strings.Contains(got, `attributeName="href"`) {
				t.Fatalf("tape used href switching: %s", got)
			}
			if animation == AnimationCSS && !strings.Contains(got, "translateY(-500px)") {
				t.Fatal("CSS vertical timeline missing")
			}
			if animation == AnimationCSS && !strings.Contains(got, " 2 step-end forwards") {
				t.Fatal("finite CSS tape does not freeze its final transform")
			}
			if animation == AnimationSMIL && (!strings.Contains(got, `<animateTransform`) || !strings.Contains(got, `values="0 0;0 -25`)) {
				t.Fatal("SMIL vertical timeline missing")
			}
			if animation == AnimationSMIL && !strings.Contains(got, `0 -500" keyTimes=`) {
				t.Fatal("SMIL terminal transform is not the final tape state")
			}
			if animation == AnimationSMIL && !strings.Contains(got, `fill="freeze"`) {
				t.Fatal("finite SMIL tape does not freeze its final transform")
			}
			if !strings.Contains(got, " 2 step-end") && !strings.Contains(got, `repeatCount="2"`) {
				t.Fatal("finite loop count missing")
			}
		})
	}
}

func TestFiniteScrollLayoutFreezesTapeFallbackBandAndCursor(t *testing.T) {
	rec := scrollRecording(120, 40, 21)
	rec.Height = 42
	for i := range rec.Frames {
		rec.Frames[i].Rows = append(rec.Frames[i].Rows, ir.Row{Y: 41, Runs: []ir.TextRun{{
			Text: string(rune('a' + i)), StartCol: 0, EndCol: 1,
		}}})
	}
	config := renderer.DefaultConfig()
	config.LoopCount = 2
	assertSemanticParityWithConfig(t, rec, config, WithLayout(LayoutScroll))

	var out bytes.Buffer
	if err := New(config, WithLayout(LayoutScroll)).Render(context.Background(), rec, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if count := strings.Count(got, "step-end forwards"); count != 3 {
		t.Fatalf("finite fill count = %d, want tape, fallback band, and cursor: %s", count, got)
	}
	if !strings.Contains(got, `animation:cursor 20s 2 step-end forwards`) {
		t.Fatal("moving cursor does not retain its terminal state")
	}

	var legacy bytes.Buffer
	if err := New(config, WithLayout(LayoutBands)).Render(context.Background(), rec, &legacy); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(legacy.String(), "step-end forwards") {
		t.Fatal("finite fill changed the legacy band layout")
	}
}

func TestExactBandReplacementDeltaIsAdditiveAndKeepsSnapshotOnTie(t *testing.T) {
	snapshot := preparedContentCost{definitions: 100, styles: 40, active: 60, regionBytes: 999}
	tape := preparedContentCost{definitions: 90, styles: 45, active: 50, regionBytes: 1}
	if got := exactBandReplacementDelta(snapshot, tape); got != -15 {
		t.Fatalf("replacement delta = %d, want -15", got)
	}

	// An unchanged neighboring band contributes equally to both complete
	// ledgers and must cancel out of the candidate band's replacement delta.
	snapshot.definitions += 73
	tape.definitions += 73
	if got := exactBandReplacementDelta(snapshot, tape); got != -15 {
		t.Fatalf("replacement delta with shared neighbor = %d, want -15", got)
	}

	tie := snapshot
	if scrollTapeWins(snapshot, tie) {
		t.Fatal("zero replacement delta selected the tape")
	}
}

func TestScrollAndRectTracksChooseStrictSmallestFromSnapshot(t *testing.T) {
	snapshot := preparedContent{bands: []preparedBand{{kind: bandSnapshot}}, cost: preparedContentCost{regionBytes: 100}}
	retained := preparedContent{bands: []preparedBand{{kind: bandRetainedRect}}, cost: preparedContentCost{regionBytes: 90}}
	tape := preparedContent{bands: []preparedBand{{kind: bandScrollTape}}, cost: preparedContentCost{regionBytes: 80}}
	if got := strictSmallestPrepared(snapshot, retained, tape); got.bands[0].kind != bandScrollTape {
		t.Fatalf("selected kind = %d, want scroll tape", got.bands[0].kind)
	}
	if got := strictSmallestPrepared(snapshot, preparedContent{bands: retained.bands, cost: snapshot.cost},
		preparedContent{bands: tape.bands, cost: snapshot.cost}); got.bands[0].kind != bandSnapshot {
		t.Fatalf("tie selected kind = %d, want snapshot", got.bands[0].kind)
	}
}

func TestScrollTapeCandidateRestoresSnapshotBandBeforeConversion(t *testing.T) {
	snapshot := preparedBand{kind: bandSnapshot, width: 4, height: 2, content: timeline[[]ir.Row]{duration: time.Second}}
	tape := scrollTape{rows: []ir.Row{{Y: 0}}, offsets: []keyframePoint[int]{{state: 0}, {selector: "100%", state: 1}}}
	got := scrollTapeCandidateBand(&snapshot, tape)
	if got.kind != bandScrollTape || got.track != nil || got.content.duration != snapshot.content.duration || got.tapeHeight != 1 {
		t.Fatalf("scroll candidate = %#v", got)
	}
}

func TestExactBandReplacementDeltaRebuildsSharedDefinitions(t *testing.T) {
	rec := scrollRecording(120, 40, 21)
	config := renderer.DefaultConfig()
	plan, err := buildSemanticPlan(context.Background(), rec, false, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := scrollCanvas(rec, &plan, config)
	source := buildRowBands(&plan, plan.width, plan.height)[0]
	snapshot, err := c.prepareLocalViewports(context.Background(), []rowBand{source, source})
	if err != nil {
		t.Fatal(err)
	}
	tape, ok := detectUpwardScrollTape(source, rec.Colors)
	if !ok {
		t.Fatal("fixture did not produce a strict tape")
	}
	bands := slices.Clone(snapshot.bands)
	bands[0].kind = bandScrollTape
	bands[0].tapeRaw = tape.rows
	bands[0].offsets = tape.offsets
	bands[0].tapeHeight = len(tape.rows)
	candidate, err := c.materializeBands(context.Background(), bands)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.bands[1].content, candidate.bands[1].content) {
		t.Fatal("candidate replacement changed the neighboring band")
	}
	want := candidate.cost.regionBytes - snapshot.cost.regionBytes
	if got := exactBandReplacementDelta(snapshot.cost, candidate.cost); got != want {
		t.Fatalf("replacement delta with rebuilt shared definitions = %d, want %d", got, want)
	}
}

func TestScrollTapeMetricsAndExactCost(t *testing.T) {
	rec := scrollRecording(120, 40, 21)
	config := renderer.DefaultConfig()
	metrics, err := New(config, WithLayout(LayoutScroll)).MeasureCandidate(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.ScrollTapeCount != 1 || metrics.MaxTapeWidth != 972 || metrics.MaxTapeHeight != 1500 || metrics.MaxTapeArea != 1458000 || metrics.ScrollTransformAnimations != 1 {
		t.Fatalf("scroll metrics = %#v", metrics)
	}
	assertCandidateCostExact(t, rec, config, &Options{Layout: LayoutScroll, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate, AutoObjective: AutoObjectiveSize, Style: StyleLegacy})
}

func scrollCanvas(rec *ir.Recording, plan *renderPlan, config *renderer.Config) canvas {
	return canvas{rec: rec, plan: *plan, config: *config, options: Options{Layout: LayoutScroll, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate, AutoObjective: AutoObjectiveSize, Style: StyleLegacy}, classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{}}
}

func scrollRecording(width, height, states int) *ir.Recording {
	catalog := termcolor.NewCatalog(stdcolor.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, stdcolor.RGBA{A: 0xff})
	red := catalog.Register(termcolor.FromRGB(0xff, 0, 0), nil)
	rec := &ir.Recording{Width: width, Height: height, Duration: time.Duration(states-1) * time.Second, Colors: catalog}
	for state := range states {
		rows := make([]ir.Row, height)
		for y := range height {
			text := strings.Repeat(string(rune('!'+state+y)), 80)
			attrs := ir.CellAttrs{}
			if state+y == 42 {
				text = ""
			}
			if state+y == 43 {
				text, attrs = " ", ir.CellAttrs{BG: red}
			}
			if state+y == 44 {
				text, attrs = " ", ir.CellAttrs{Underline: true}
			}
			if state+y == 45 {
				text = "界\x00"
			}
			rows[y] = ir.Row{Y: y}
			if text != "" {
				rows[y].Runs = []ir.TextRun{{Text: text, StartCol: 0, EndCol: len([]rune(text)), Attrs: attrs}}
			}
		}
		cursorState := state
		if state == states-1 && state > 0 {
			cursorState--
		}
		rec.Frames = append(rec.Frames, ir.Frame{Time: time.Duration(state) * time.Second, Rows: rows, Cursor: ir.Cursor{Col: cursorState % width, Row: (cursorState * 3) % height, Visible: true}})
	}
	return rec
}

func cloneScrollRows(rows []ir.Row) []ir.Row {
	out := make([]ir.Row, len(rows))
	for i := range rows {
		out[i] = rows[i]
		out[i].Runs = append([]ir.TextRun(nil), rows[i].Runs...)
	}
	return out
}

func reverseScrollFrames(rec *ir.Recording) {
	for left, right := 0, len(rec.Frames)-1; left < right; left, right = left+1, right-1 {
		rec.Frames[left].Rows, rec.Frames[right].Rows = rec.Frames[right].Rows, rec.Frames[left].Rows
	}
}
