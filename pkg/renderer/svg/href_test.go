package svg

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestHrefFrameSwitchRequiresSMIL(t *testing.T) {
	options := DefaultOptions()
	options.FrameSwitch = FrameSwitchHref
	if err := options.Validate(); err == nil {
		t.Fatal("href switching with CSS unexpectedly validated")
	}
	options.Animation = AnimationSMIL
	if err := options.Validate(); err != nil {
		t.Fatalf("href switching with SMIL failed validation: %v", err)
	}
}

func TestWriteSMILHrefUsesOneDiscreteReferenceTimeline(t *testing.T) {
	var out strings.Builder
	c := canvas{plan: renderPlan{duration: time.Second}}
	frames := []keyframePoint[int]{
		{selector: "0%", state: 0},
		{selector: "50%", state: 1},
		{selector: "100%", state: 0},
	}

	c.writeSMILHref(&out, frames, []string{"_f0", "_f1"})

	got := out.String()
	for _, want := range []string{
		`<animate attributeName="href"`,
		`values="#_f0;#_f1;#_f0"`,
		`keyTimes="0;.5;1"`,
		`calcMode="discrete"`,
		`repeatCount="indefinite"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("SMIL href output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "animateTransform") {
		t.Fatalf("href switching emitted a transform animation: %q", got)
	}
}

func TestWriteHrefSequenceUsesSingleRuntimeUse(t *testing.T) {
	var out strings.Builder
	c := canvas{w: &out, plan: renderPlan{duration: time.Second}}
	frames := []keyframePoint[int]{{selector: "0%", state: 1}, {selector: "100%", state: 0}}

	c.writeHrefSequence(frames, []string{"_f0", "_f1"}, "")

	got := out.String()
	if strings.Count(got, "<use ") != 1 || !strings.HasPrefix(got, `<use href="#_f1">`) {
		t.Fatalf("href sequence = %q; want one use starting at _f1", got)
	}
}

func TestWriteStateDefinitionElidesOnlyOneGeneratedChild(t *testing.T) {
	textRow := func(text, svg string) *renderedRow {
		return &renderedRow{row: ir.Row{Runs: []ir.TextRun{{Text: text}}}, svg: svg}
	}
	tests := []struct {
		name   string
		minify bool
		rows   []*renderedRow
		want   string
	}{
		{name: "empty", rows: nil, want: `<g id="_f0"></g>`},
		{name: "empty minified", minify: true, rows: nil, want: `<g id="_f0"/>`},
		{name: "text", rows: []*renderedRow{textRow("x", `<text y="20">x</text>`)}, want: `<text id="_f0" y="20">x</text>`},
		{
			name: "rect",
			rows: []*renderedRow{textRow("x", `<rect y="0" width="12"/>`)},
			want: `<rect id="_f0" y="0" width="12"/>`,
		},
		{
			name: "multiple",
			rows: []*renderedRow{textRow("x", `<text>x</text>`), textRow("y", `<text>y</text>`)},
			want: `<g id="_f0"><text>x</text><text>y</text></g>`,
		},
		{name: "row use", rows: []*renderedRow{{id: "a"}}, want: `<use id="_f0" href="#a"/>`},
		{
			name: "existing child id",
			rows: []*renderedRow{textRow("x", `<text id="row">x</text>`)},
			want: `<g id="_f0"><text id="row">x</text></g>`,
		},
		{
			name: "existing child id minified", minify: true,
			rows: []*renderedRow{textRow("x", `<text id="row">x</text>`)},
			want: `<g id="_f0"><text id="row">x</text></g>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			c := canvas{w: &out, rec: createTestRecording(), config: renderer.Config{Minify: tt.minify}}
			c.writeStateDefinition("_f0", tt.rows)
			if got := out.String(); got != tt.want {
				t.Fatalf("state definition = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestSingleChildStateElisionSavesExactBytes(t *testing.T) {
	const before = `<g id="_f0"><text y="20">x</text></g>`
	var out strings.Builder
	c := canvas{w: &out, rec: createTestRecording()}
	c.writeStateDefinition("_f0", []*renderedRow{
		{row: ir.Row{Runs: []ir.TextRun{{Text: "x"}}}, svg: `<text y="20">x</text>`},
	})
	if delta := len(before) - out.Len(); delta != 7 {
		t.Fatalf("single-child state byte delta = %d; want 7", delta)
	}
}

func TestStateIDsContinueCompactAlphabetAfterRowDefinitions(t *testing.T) {
	got, next := compactXMLIDs(xmlIDIndexAfter(25), 3)
	want := []string{"z", "aa", "ab"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("compactXMLIDs()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
	if next != 28 {
		t.Fatalf("next allocator index = %d; want 28", next)
	}
}

func TestCompactXMLIDsSkipReservedClipIdentifier(t *testing.T) {
	clipIndex := 0
	for compactXMLIDAt(clipIndex) != "clip" {
		clipIndex++
	}
	ids, next := compactXMLIDs(clipIndex-1, 3)
	if ids[0] != compactXMLIDAt(clipIndex-1) || ids[1] == "clip" || ids[1] != compactXMLIDAt(clipIndex+1) ||
		ids[2] != compactXMLIDAt(clipIndex+2) || next != clipIndex+3 {
		t.Fatalf("compactXMLIDs() around clip = %q, next %d", ids, next)
	}
	if got := xmlIDIndexAfter(clipIndex + 1); got != clipIndex+2 {
		t.Fatalf("xmlIDIndexAfter() across clip = %d; want %d", got, clipIndex+2)
	}
}

func TestStateIDsAliasSingleRowAndEmptyStates(t *testing.T) {
	row := &renderedRow{id: "c"}
	inline := &renderedRow{row: ir.Row{Runs: []ir.TextRun{{Text: "x"}}}, svg: `<text>x</text>`}
	states := [][]*renderedRow{nil, {row}, {inline}, nil, {row, inline}, {row}}
	ids, next := compactStateIDs(3, states)
	want := []string{"d", "c", "e", "d", "f", "c"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("compactStateIDs() = %q; want %q", ids, want)
		}
	}
	if next != 6 {
		t.Fatalf("next allocator index = %d; want 6", next)
	}
	owned := make([]bool, len(states))
	for i := range states {
		owned[i] = ownsStateDefinition(ids, states, i)
	}
	if !reflect.DeepEqual(owned, []bool{true, false, true, false, true, false}) {
		t.Fatalf("owned definitions = %v", owned)
	}
	if got := ownedStateDefinitions(ids, states); got != 3 {
		t.Fatalf("ownedStateDefinitions() = %d; want 3", got)
	}
}

func TestRender_AliasesSingleRowStatesToRowDefinitions(t *testing.T) {
	rec := createTestRecording()
	long := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: "ROW:" + strings.Repeat("x", 60)}}}
	other := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: "OTHER:" + strings.Repeat("y", 60)}}}
	other.Y = 1
	rec.Frames = []ir.Frame{
		{Rows: nil},
		{Time: 500 * time.Millisecond, Rows: []ir.Row{long}},
		{Time: time.Second, Rows: []ir.Row{long, other}},
		{Time: 1500 * time.Millisecond, Rows: []ir.Row{other}},
		{Time: 2 * time.Second, Rows: nil},
		{Time: 2500 * time.Millisecond, Rows: []ir.Row{long, other}},
	}
	rec.Duration = 3 * time.Second

	var buf bytes.Buffer
	r := New(renderer.DefaultConfig(), WithAnimation(AnimationSMIL), WithFrameSwitch(FrameSwitchHref))
	if err := r.Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	if strings.Contains(svg, `<use id="`) {
		t.Fatalf("single-row states still wrap their row reference:\n%s", svg)
	}
	// Rows: long = a, other = b (both interned). States: blank = c (its own
	// definition), [long] = a, [long, other] = d, [other] = b; the blank state
	// recurs and the final keyframe holds the last state.
	if strings.Count(svg, `<g id="c"></g>`) != 1 || strings.Count(svg, `<g id="`) != 2 ||
		!strings.Contains(svg, `<g id="d"><use href="#a"/><use href="#b"/></g>`) {
		t.Fatalf("state definitions are not shared as expected:\n%s", svg)
	}
	if !strings.Contains(svg, `values="#c;#a;#d;#b;#c;#d;#d"`) {
		t.Fatalf("href values do not reference the row definitions directly:\n%s", svg)
	}
	assertSemanticParity(t, rec, WithAnimation(AnimationSMIL), WithFrameSwitch(FrameSwitchHref))
}
