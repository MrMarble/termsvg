package svg

import (
	"bytes"
	"context"
	"encoding/xml"
	"image/color"
	"reflect"
	"strings"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

type rectState struct {
	x, width, secondX, secondWidth int
	alternate                      bool
}

func TestRectTracksEmitProfitableGrowingBar(t *testing.T) {
	states := make([]rectState, 24)
	for i := 1; i < len(states)-1; i++ {
		states[i] = rectState{x: i % 3, width: i}
	}
	states[12] = rectState{}
	states[len(states)-1] = rectState{x: 2, width: 22}
	rec := rectTrackRecording(t, states, false)
	before := cloneRecording(rec)
	config := renderer.DefaultConfig()
	config.Minify = true
	config.LoopCount = 2
	r := New(config, WithLayout(LayoutRegions), WithAnimation(AnimationSMIL), WithPrimitiveMode(PrimitiveRectTracks))

	var out bytes.Buffer
	if err := r.Render(context.Background(), rec, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if err := xml.Unmarshal(out.Bytes(), new(struct{})); err != nil {
		t.Fatalf("invalid SVG XML: %v", err)
	}
	var repeated bytes.Buffer
	if err := r.Render(context.Background(), rec, &repeated); err != nil || repeated.String() != got {
		t.Fatalf("retained output is nondeterministic: %v", err)
	}
	for _, want := range []string{
		`<animate attributeName="x"`, `<animate attributeName="width"`, `calcMode="discrete"`,
		`fill="freeze"`, `repeatCount="2"`, `keyTimes="0;.04167;.08333`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("retained bar missing %q: %s", want, got)
		}
	}
	if strings.Index(got, `<rect class=`) > strings.Index(got, `<text`) && strings.Contains(got, `<text`) {
		t.Fatalf("retained rectangle follows text: %s", got)
	}
	metrics, err := r.MeasureCandidate(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RetainedPrimitiveCount != 1 || metrics.GeometryAnimationNodes != 2 ||
		metrics.PaintPropertyAnimationNodes != 0 {
		t.Fatalf("retained metrics = %#v", metrics)
	}
	if !reflect.DeepEqual(rec, before) {
		t.Fatal("renderer mutated caller-owned IR")
	}
}

func TestRectTracksEmitProfitableShrinkingBar(t *testing.T) {
	states := make([]rectState, 24)
	for i := range states {
		states[i] = rectState{width: len(states) - i}
	}
	config := renderer.DefaultConfig()
	config.Minify = true
	metrics, err := New(config, WithLayout(LayoutScroll), WithAnimation(AnimationSMIL),
		WithPrimitiveMode(PrimitiveRectTracks)).MeasureCandidate(context.Background(), rectTrackRecording(t, states, false))
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RetainedPrimitiveCount != 1 || metrics.GeometryAnimationNodes != 1 {
		t.Fatalf("shrinking bar metrics = %#v", metrics)
	}
}

func TestRectTracksAnimateFillAndKeepTextAboveBackground(t *testing.T) {
	// Every state is distinct (growing width, alternating colour) so the
	// snapshot strip cannot fold the timeline into two shared states.
	states := make([]rectState, 20)
	for i := range states {
		states[i] = rectState{width: 8 + i, alternate: i%2 == 1}
	}
	rec := rectTrackRecording(t, states, true)
	config := renderer.DefaultConfig()
	config.Minify = true
	r := New(config, WithLayout(LayoutRegions), WithAnimation(AnimationSMIL), WithPrimitiveMode(PrimitiveRectTracks))
	var out bytes.Buffer
	if err := r.Render(context.Background(), rec, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `<animate attributeName="fill"`) {
		t.Fatalf("fill track missing: %s", got)
	}
	if strings.Index(got, `<rect`) > strings.LastIndex(got, `<text`) {
		t.Fatalf("background painter order changed: %s", got)
	}
}

func TestRectTracksFallBackOnAmbiguousRectangleIdentity(t *testing.T) {
	rec := rectTrackRecording(t, []rectState{{width: 8}, {width: 3, secondX: 5, secondWidth: 3}, {width: 8}}, false)
	r := New(renderer.DefaultConfig(),
		WithLayout(LayoutRegions), WithAnimation(AnimationSMIL), WithPrimitiveMode(PrimitiveRectTracks))
	metrics, err := r.MeasureCandidate(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RetainedPrimitiveCount != 0 {
		t.Fatalf("ambiguous spans produced %d tracks", metrics.RetainedPrimitiveCount)
	}
}

func TestRectTracksKeepIndependentAdjacentRegions(t *testing.T) {
	rec := adjacentRectTrackRecording(t)
	config := renderer.DefaultConfig()
	config.Minify = true
	metrics, err := New(config, WithLayout(LayoutRegions), WithAnimation(AnimationSMIL),
		WithPrimitiveMode(PrimitiveRectTracks)).MeasureCandidate(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RetainedPrimitiveCount < 2 {
		t.Fatalf("retained primitives = %d, want independent tracks", metrics.RetainedPrimitiveCount)
	}
}

func TestRetainedRectTieKeepsSnapshot(t *testing.T) {
	cost := preparedContentCost{definitions: 1, styles: 2, active: 3}
	if retainedRectWins(cost, cost) {
		t.Fatal("equal retained representation replaced snapshots")
	}
}

func TestRetainedRectEmitsCompleteExactTimelines(t *testing.T) {
	catalog := termcolor.NewCatalog(color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{A: 255})
	red := catalog.Register(termcolor.FromRGB(180, 20, 20), nil)
	blue := catalog.Register(termcolor.FromRGB(20, 80, 180), nil)
	row := func(x, width int, bg termcolor.ID) []ir.Row {
		if width == 0 {
			return []ir.Row{{Y: 0}}
		}
		run := ir.TextRun{Text: strings.Repeat(" ", width), StartCol: x, EndCol: x + width, Attrs: ir.CellAttrs{BG: bg}}
		return []ir.Row{{Y: 0, Runs: []ir.TextRun{run}}}
	}
	content := timeline[[]ir.Row]{duration: 4 * time.Second, points: []timelinePoint[[]ir.Row]{
		{time: 0, state: row(0, 0, red)},
		{time: time.Second, state: row(1, 2, red)},
		{time: 2 * time.Second, state: row(3, 4, blue)},
		{time: 3 * time.Second, state: row(0, 0, blue)},
		{time: 4 * time.Second, state: row(0, 0, blue)},
	}}
	c := canvas{
		rec: &ir.Recording{Colors: catalog}, plan: renderPlan{duration: 4 * time.Second},
		config: renderer.Config{LoopCount: 2}, classNames: catalog.GenerateClassNames(),
		style: stylePlan{scheme: styleLegacy},
	}
	track, _, ok := c.retainedRectCandidate(rowBand{height: 1, content: content})
	if !ok {
		t.Fatal("exact rectangle fixture was rejected")
	}
	var out strings.Builder
	c.w = &out
	c.writeRetainedRect(&track)
	for _, want := range []string{
		`<animate attributeName="x" values="12;12;36;36;36" keyTimes="0;.25;.5;.75;1" ` +
			`calcMode="discrete" dur="4s" repeatCount="2" fill="freeze"/>`,
		`<animate attributeName="width" values="0;24;48;0;0" keyTimes="0;.25;.5;.75;1" ` +
			`calcMode="discrete" dur="4s" repeatCount="2" fill="freeze"/>`,
		`<animate attributeName="fill" values="#B41414;#B41414;#1450B4;#1450B4;#1450B4" keyTimes="0;.25;.5;.75;1" ` +
			`calcMode="discrete" dur="4s" repeatCount="2" fill="freeze"/>`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("exact retained timeline missing %s in %s", want, out.String())
		}
	}
}

func rectTrackRecording(tb testing.TB, states []rectState, text bool) *ir.Recording {
	tb.Helper()
	catalog := termcolor.NewCatalog(color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{A: 255})
	red := catalog.Register(termcolor.FromRGB(180, 20, 20), nil)
	blue := catalog.Register(termcolor.FromRGB(20, 80, 180), nil)
	frames := make([]ir.Frame, len(states))
	for i, state := range states {
		bg := red
		if state.alternate {
			bg = blue
		}
		var runs []ir.TextRun
		if state.width > 0 {
			value := strings.Repeat(" ", state.width)
			if text {
				value = "42%" + strings.Repeat(" ", max(0, state.width-3))
			}
			runs = append(runs, ir.TextRun{
				Text: value, StartCol: state.x, EndCol: state.x + state.width, Attrs: ir.CellAttrs{BG: bg},
			})
		}
		if state.secondWidth > 0 {
			runs = append(runs, ir.TextRun{
				Text:     strings.Repeat(" ", state.secondWidth),
				StartCol: state.secondX, EndCol: state.secondX + state.secondWidth, Attrs: ir.CellAttrs{BG: blue},
			})
		}
		frames[i] = ir.Frame{Time: time.Duration(i) * time.Second, Rows: []ir.Row{{Y: 0, Runs: runs}}}
	}
	return &ir.Recording{
		Width: 32, Height: 1, Duration: time.Duration(len(states)) * time.Second, Frames: frames, Colors: catalog,
	}
}

func adjacentRectTrackRecording(t *testing.T) *ir.Recording {
	t.Helper()
	catalog := termcolor.NewCatalog(color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{A: 255})
	red := catalog.Register(termcolor.FromRGB(180, 20, 20), nil)
	blue := catalog.Register(termcolor.FromRGB(20, 80, 180), nil)
	frames := make([]ir.Frame, 32)
	left, right := 1, 1
	for i := range frames {
		if i%2 == 0 {
			left++
		} else {
			right++
		}
		frames[i] = ir.Frame{Time: time.Duration(i) * time.Second, Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{
			{Text: strings.Repeat(" ", left), EndCol: left, Attrs: ir.CellAttrs{BG: red}},
			{Text: strings.Repeat(" ", right), StartCol: 20, EndCol: 20 + right, Attrs: ir.CellAttrs{BG: blue}},
		}}}}
	}
	return &ir.Recording{Width: 40, Height: 1, Duration: 32 * time.Second, Frames: frames, Colors: catalog}
}
