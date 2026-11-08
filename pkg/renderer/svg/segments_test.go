package svg

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func promptRuns(rec *ir.Recording, command string) []ir.TextRun {
	palette := termcolor.Standard()
	green := rec.Colors.Register(termcolor.FromRGB(0, 255, 0), &palette)
	blue := rec.Colors.Register(termcolor.FromRGB(0, 0, 255), &palette)
	runs := []ir.TextRun{
		parityRun("user@host", 0, ir.CellAttrs{FG: green, Bold: true}),
		parityRun(":", 9, ir.CellAttrs{}),
		parityRun("~/projects/termsvg", 10, ir.CellAttrs{FG: blue, Bold: true}),
		parityRun("$", 29, ir.CellAttrs{}),
	}
	if command != "" {
		runs = append(runs, parityRun(command, 31, ir.CellAttrs{}))
	}
	return runs
}

// promptRecording redraws a shell prompt with a growing command on the same
// row, then repeats the prompt on later rows after scrolling.
func promptRecording() *ir.Recording {
	rec := parityRecording(60, 4, nil)
	states := make([][]ir.Row, 0, 8)
	for _, command := range []string{"", "l", "ls", "ls ", "ls -l"} {
		states = append(states, []ir.Row{parityRow(0, promptRuns(rec, command)...)})
	}
	total := parityRow(1, parityRun("total 0", 0, ir.CellAttrs{}))
	states = append(states,
		[]ir.Row{parityRow(0, promptRuns(rec, "ls -l")...), total, parityRow(2, promptRuns(rec, "")...)},
		[]ir.Row{parityRow(0, promptRuns(rec, "ls -l")...), total, parityRow(2, promptRuns(rec, "exit")...)},
		[]ir.Row{
			parityRow(0, parityRun("total 0", 0, ir.CellAttrs{})),
			parityRow(1, promptRuns(rec, "exit")...),
			parityRow(3, promptRuns(rec, "")...),
		},
	)
	rec.Frames = make([]ir.Frame, len(states))
	for i, rows := range states {
		rec.Frames[i] = ir.Frame{
			Time: time.Duration(i) * time.Second, Rows: rows, Cursor: ir.Cursor{Col: 31, Row: 0, Visible: true},
		}
	}
	rec.Duration = time.Duration(len(states)) * time.Second
	return rec
}

func TestSegmentSharingFactorsRepeatedPromptPrefix(t *testing.T) {
	rec := promptRecording()
	var buf bytes.Buffer
	// The last two states also recur as a row block; this test inspects the
	// segment references of the rows themselves.
	if err := New(renderer.DefaultConfig(), withoutBlockSharing).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	if got := strings.Count(svg, ">user@host</text>"); got != 1 {
		t.Fatalf("prompt serialized %d times, want 1:\n%s", got, svg)
	}
	if !strings.Contains(svg, `<g id="a"><text y="20" class="`) {
		t.Fatalf("prompt prefix was not defined as the first shared segment:\n%s", svg)
	}
	if !strings.Contains(svg, `<use href="#a"/>`) || !strings.Contains(svg, `<use href="#a" y="50"/>`) ||
		!strings.Contains(svg, `<use href="#a" y="25"/>`) || !strings.Contains(svg, `<use href="#a" y="75"/>`) {
		t.Fatalf("prompt references do not carry the row offsets:\n%s", svg)
	}
	if strings.Contains(svg, `<use href="#a"/><text x="372"`) == false {
		t.Fatalf("command text does not follow the shared prompt inline:\n%s", svg)
	}
}

func TestSegmentSharingPreservesSemanticsAcrossLayouts(t *testing.T) {
	rec := promptRecording()
	for _, variant := range parityOptions {
		t.Run(variant.name, func(t *testing.T) { assertSemanticParity(t, rec, variant.options...) })
	}
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions, LayoutScroll} {
		t.Run(string(layout)+"/auto-style", func(t *testing.T) {
			assertSemanticParity(t, rec, WithLayout(layout), WithStyleMode(StyleAuto))
		})
	}
}

func TestSegmentSharingMetricsMatchSerializedStructure(t *testing.T) {
	rec := promptRecording()
	for _, variant := range parityOptions {
		for _, style := range []StyleMode{StyleLegacy, StyleAuto} {
			options := append(slices.Clone(variant.options), WithStyleMode(style))
			t.Run(variant.name+"/"+string(style), func(t *testing.T) {
				assertSharedSegmentMetrics(t, rec, options)
			})
		}
	}
}

func assertSharedSegmentMetrics(t *testing.T, rec *ir.Recording, opts []Option) {
	t.Helper()
	config := renderer.DefaultConfig()
	options := DefaultOptions()
	for _, apply := range opts {
		apply(&options)
	}
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, options.MaxFPS, config.LoopCount)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := prepareCandidate(context.Background(), rec, &plan, config, &options)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	parsed := CandidateMetrics{}
	writer := &candidateWriter{w: &out, metrics: &parsed}
	if err := New(config, opts...).serializeCandidate(context.Background(), rec, writer, candidate); err != nil {
		t.Fatal(err)
	}
	writer.finish()
	if !strings.Contains(out.String(), `<use href="#a"`) {
		t.Fatalf("segment sharing did not apply:\n%s", out.String())
	}
	got := candidate.metrics
	if candidate.cost.finalBytes != parsed.FinalBytes {
		t.Fatalf("prepared bytes = %d, serialized %d", candidate.cost.finalBytes, parsed.FinalBytes)
	}
	if got.XMLNodes != parsed.XMLNodes || got.DefinitionNodes != parsed.DefinitionNodes ||
		got.ActiveNodes != parsed.ActiveNodes || got.TextNodes != parsed.TextNodes ||
		got.RectNodes != parsed.RectNodes || got.GroupNodes != parsed.GroupNodes ||
		got.UseNodes != parsed.UseNodes || got.AnimationNodes != parsed.AnimationNodes {
		t.Fatalf("prepared metrics = %#v; serialized = %#v", got, parsed)
	}
	if got.MaxUseDepth < 1 || got.StaticUseShadowNodes == 0 && got.PeakAnimatedUseShadowNodes == 0 {
		t.Fatalf("use expansion metrics ignore shared segments: %#v", got)
	}
}

func TestSegmentSharingSharesRowsAtOtherPositions(t *testing.T) {
	rec := parityRecording(40, 3, [][]ir.Row{
		{parityRow(0, parityRun("a long line of scrolling output", 2, ir.CellAttrs{}))},
		{parityRow(1, parityRun("a long line of scrolling output", 2, ir.CellAttrs{}))},
		{parityRow(2, parityRun("a long line of scrolling output", 4, ir.CellAttrs{}))},
	})
	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	if strings.Count(svg, "scrolling output") != 1 {
		t.Fatalf("moving line serialized more than once:\n%s", svg)
	}
	for _, want := range []string{
		`<text id="a" x="24" y="20">`, `<use href="#a"/>`, `<use href="#a" y="25"/>`, `<use href="#a" x="24" y="50"/>`,
	} {
		if !strings.Contains(svg, want) {
			t.Fatalf("missing %q:\n%s", want, svg)
		}
	}
	assertSemanticParity(t, rec)
}

func TestSegmentSharingKeepsMergedBackgroundsIntact(t *testing.T) {
	rec := parityRecording(40, 2, nil)
	palette := termcolor.Standard()
	bg := rec.Colors.Register(termcolor.FromRGB(30, 40, 50), &palette)
	fg := rec.Colors.Register(termcolor.FromRGB(200, 100, 50), &palette)
	shared := func(suffix string) []ir.TextRun {
		return []ir.TextRun{
			parityRun("powerline", 0, ir.CellAttrs{BG: bg}),
			parityRun("segment", 9, ir.CellAttrs{BG: bg, FG: fg}),
			parityRun(suffix, 16, ir.CellAttrs{BG: bg}),
		}
	}
	rec.Frames = []ir.Frame{
		{Rows: []ir.Row{parityRow(0, shared("first tail")...)}},
		{Time: time.Second, Rows: []ir.Row{parityRow(0, shared("second tail")...)}},
		{Time: 2 * time.Second, Rows: []ir.Row{parityRow(1, shared("third tail")...)}},
	}
	rec.Duration = 3 * time.Second
	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	if strings.Contains(svg, `<use href="#`) {
		t.Fatalf("a merged background rectangle was split by a shared segment:\n%s", svg)
	}
	if strings.Count(svg, ">powerline</text>") != 3 {
		t.Fatalf("rows with one merged background were not serialized inline:\n%s", svg)
	}
	assertSemanticParity(t, rec)
}

func TestSegmentSharingRejectsUnprofitableFragments(t *testing.T) {
	rec := parityRecording(20, 3, [][]ir.Row{
		{parityRow(0, parityRun("|", 3, ir.CellAttrs{})), parityRow(1, parityRun("|", 3, ir.CellAttrs{}))},
		{parityRow(1, parityRun("|", 3, ir.CellAttrs{})), parityRow(2, parityRun("|", 3, ir.CellAttrs{}))},
	})
	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), `<use href="#`) {
		t.Fatalf("one-character fragments were shared at a byte loss:\n%s", buf.String())
	}
}

func TestSegmentSavingsUseExactMarkupBytes(t *testing.T) {
	rec := promptRecording()
	c := &canvas{
		rec: rec, config: *renderer.DefaultConfig(), classNames: rec.Colors.GenerateClassNames(),
		options: Options{withoutBlockSharing: true},
	}
	c.plan = buildRenderPlan(rec, false)
	_, states := c.contentKeyframes()
	frames, defs := c.collectRows(states)
	if len(defs) == 0 || defs[0].id != "a" || len(defs[0].uses) != 0 {
		t.Fatalf("shared prompt definition missing: %#v", defs)
	}
	prefix := defs[0]
	var expected strings.Builder
	c.writeRow(&expected, prefix.row)
	if prefix.svg != expected.String() || prefix.definition != `<g id="a">`+prefix.svg+`</g>` {
		t.Fatalf("definition markup = %q; want anchored row %q", prefix.definition, expected.String())
	}
	if prefix.elements != 4 || c.renderedElementCount(prefix) != 4 {
		t.Fatalf("prefix elements = %d/%d; want 4", prefix.elements, c.renderedElementCount(prefix))
	}
	for _, rows := range frames {
		for _, row := range rows {
			if len(row.uses) == 0 {
				continue
			}
			if row.uses[0] != "a" || len(row.paintRow().Runs) != len(row.row.Runs)-4 {
				t.Fatalf("row %#v does not reference the prompt and paint only its command", row)
			}
			if got := strings.Count(row.svg, "<"); got != row.elements+strings.Count(row.svg, "</") {
				t.Fatalf("row elements = %d for %q", row.elements, row.svg)
			}
		}
	}
}
