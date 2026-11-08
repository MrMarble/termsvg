//nolint:lll // Compact comparator cases show each complete metric change.
package svg

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mrmarble/termsvg/internal/svgoutput"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestLayoutAutoValidatesWithoutChangingDefault(t *testing.T) {
	if DefaultOptions().Layout != LayoutFrames {
		t.Fatalf("default layout = %q; want frames", DefaultOptions().Layout)
	}
	options := DefaultOptions()
	options.Layout = LayoutAuto
	if err := options.Validate(); err != nil {
		t.Fatalf("auto layout failed validation: %v", err)
	}
}

func TestCountingWriterCountsAllBytes(t *testing.T) {
	counter := &countingWriter{}
	for _, value := range []string{"abc", "", "defgh"} {
		if _, err := counter.Write([]byte(value)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if counter.size() != 8 {
		t.Fatalf("counted bytes = %d; want 8", counter.size())
	}
}

func TestCountingWriterUsesPostMinifyNBSPWidth(t *testing.T) {
	counter := &countingWriter{collapseNBSP: true}
	if _, err := counter.Write([]byte{'a', 0xc2}); err != nil {
		t.Fatal(err)
	}
	if _, err := counter.Write([]byte{0xa0, 'b'}); err != nil {
		t.Fatal(err)
	}
	if counter.size() != int64(len("a b")) {
		t.Fatalf("counted transformed bytes = %d; want %d", counter.size(), len("a b"))
	}
}

func TestPreparedCandidatesShareOneImmutableSemanticPlan(t *testing.T) {
	rec := experimentalRecording()
	plan, err := buildSemanticPlan(context.Background(), rec, true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := cloneSemanticPlan(&plan)
	config := renderer.DefaultConfig()

	frameOptions := DefaultOptions()
	frames, err := prepareCandidate(context.Background(), rec, &plan, config, &frameOptions)
	if err != nil {
		t.Fatal(err)
	}
	bandOptions := DefaultOptions()
	bandOptions.Layout = LayoutBands
	bands, err := prepareCandidate(context.Background(), rec, &plan, config, &bandOptions)
	if err != nil {
		t.Fatal(err)
	}

	if frames.plan != &plan || bands.plan != &plan {
		t.Fatal("prepared candidates do not share the semantic plan")
	}
	if !reflect.DeepEqual(plan, before) {
		t.Fatal("candidate preparation mutated the semantic plan")
	}
}

func TestSelectPreparedCandidateKeepsFramesOnTie(t *testing.T) {
	frames := preparedCandidate{options: Options{Layout: LayoutFrames}, metrics: CandidateMetrics{FinalBytes: 10}}
	bands := preparedCandidate{options: Options{Layout: LayoutBands}, metrics: CandidateMetrics{FinalBytes: 10}}
	regions := preparedCandidate{options: Options{Layout: LayoutRegions}, metrics: CandidateMetrics{FinalBytes: 10}}
	if got := selectPreparedCandidate(AutoObjectiveSize, &frames, &bands, &regions); got != &frames {
		t.Fatalf("tie selected %q; want frames", got.options.Layout)
	}
	bands.metrics.FinalBytes = 9
	regions.metrics.FinalBytes = 9
	if got := selectPreparedCandidate(AutoObjectiveSize, &frames, &bands, &regions); got != &bands {
		t.Fatalf("band/region tie selected %q; want bands", got.options.Layout)
	}
}

func TestRuntimeCandidateComparator(t *testing.T) {
	base := CandidateMetrics{
		PeakLiveNodeEstimate: 10, DurationWeightedInstantiatedNodeNanos: 20,
		UseTargetSwitches: 3, AnimationNodes: 4, AnimatedElements: 5,
		LocalViewportCount: 6, MaxViewportWidth: 7, MaxViewportHeight: 8, MaxViewportArea: 56,
		MaxTranslatedArea: 9, FinalBytes: 100,
	}
	tests := []struct {
		name        string
		left, right CandidateMetrics
		want        bool
	}{
		{name: "href expansion", left: metricWith(&base, func(m *CandidateMetrics) {
			m.FinalBytes--
			m.PeakLiveNodeEstimate++
		}), right: base, want: false},
		{name: "instantiated duration", left: metricWith(&base, func(m *CandidateMetrics) {
			m.DurationWeightedInstantiatedNodeNanos--
		}), right: base, want: true},
		{name: "target switches", left: metricWith(&base, func(m *CandidateMetrics) { m.UseTargetSwitches-- }), right: base, want: true},
		{name: "animation nodes", left: metricWith(&base, func(m *CandidateMetrics) { m.AnimationNodes-- }), right: base, want: true},
		{name: "animated elements", left: metricWith(&base, func(m *CandidateMetrics) { m.AnimatedElements-- }), right: base, want: true},
		{name: "viewport count", left: metricWith(&base, func(m *CandidateMetrics) { m.LocalViewportCount-- }), right: base, want: true},
		{name: "viewport area", left: metricWith(&base, func(m *CandidateMetrics) { m.MaxViewportArea-- }), right: base, want: true},
		{name: "translated area", left: metricWith(&base, func(m *CandidateMetrics) { m.MaxTranslatedArea-- }), right: base, want: true},
		{name: "bytes", left: metricWith(&base, func(m *CandidateMetrics) { m.FinalBytes-- }), right: base, want: true},
		{name: "fewer nodes beats far more animations", left: metricWith(&base, func(m *CandidateMetrics) {
			m.PeakLiveNodeEstimate--
			m.AnimationNodes = 1000
		}), right: base, want: true},
		{name: "complete tie", left: base, right: base, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeCandidateLess(&test.left, &test.right); got != test.want {
				t.Fatalf("runtimeCandidateLess() = %v, want %v", got, test.want)
			}
		})
	}
}

func metricWith(metric *CandidateMetrics, change func(*CandidateMetrics)) CandidateMetrics {
	changed := *metric
	change(&changed)
	return changed
}

func TestRuntimeSelectionKeepsStableOrderOnCompleteTie(t *testing.T) {
	metrics := CandidateMetrics{PeakLiveNodeEstimate: 1, FinalBytes: 1}
	frames := preparedCandidate{options: Options{Layout: LayoutFrames}, metrics: metrics}
	bands := preparedCandidate{options: Options{Layout: LayoutBands}, metrics: metrics}
	regions := preparedCandidate{options: Options{Layout: LayoutRegions}, metrics: metrics}
	if got := selectPreparedCandidate(AutoObjectiveRuntime, &frames, &bands, &regions); got != &frames {
		t.Fatalf("runtime tie selected %q; want frames", got.options.Layout)
	}
}

func TestRuntimeSelectionRejectsMeasuredHrefExpansion(t *testing.T) {
	// Pairs of adjacent rows flip together so every band state holds two rows
	// and needs its own definition; a single interned row would be referenced
	// directly and expose no nested expansion.
	states := make([][]ir.Row, 12)
	for state := range states {
		states[state] = make([]ir.Row, 2*len(states))
		for row := range states[state] {
			value := "0"
			if state >= row/2 {
				value = "1"
			}
			states[state][row] = parityRow(row, parityRun(value, row, ir.CellAttrs{}))
		}
	}
	rec := parityRecording(24, 24, states)
	config := renderer.DefaultConfig()
	config.Minify = true
	// The staircase recurs as row blocks, which makes the frames layout the
	// smaller one; the fixture exposes href expansion without them.
	withoutBlocks := func(o *Options) { o.withoutBlockSharing = true }
	frameMetrics, err := New(config, WithLayout(LayoutFrames), WithAnimation(AnimationSMIL), WithFrameSwitch(FrameSwitchHref), withoutBlocks).MeasureCandidate(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	bandMetrics, err := New(config, WithLayout(LayoutBands), WithAnimation(AnimationSMIL), WithFrameSwitch(FrameSwitchHref), withoutBlocks).MeasureCandidate(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	frames := preparedCandidate{options: Options{Layout: LayoutFrames}, metrics: frameMetrics}
	bands := preparedCandidate{options: Options{Layout: LayoutBands}, metrics: bandMetrics}
	if bands.metrics.FinalBytes >= frames.metrics.FinalBytes || bands.metrics.MaxUseDepth < 2 ||
		bands.metrics.PeakLiveNodeEstimate <= frames.metrics.PeakLiveNodeEstimate {
		t.Fatalf("fixture does not expose href expansion: frames=%#v bands=%#v", frames.metrics, bands.metrics)
	}
	if got := selectPreparedCandidate(AutoObjectiveSize, &frames, &bands); got != &bands {
		t.Fatalf("size selected %q; want smaller bands fixture", got.options.Layout)
	}
	if got := selectPreparedCandidate(AutoObjectiveRuntime, &frames, &bands); got != &frames {
		t.Fatalf("runtime selected %q; want lower-expansion frames fixture", got.options.Layout)
	}
}

func TestAutoDebugLogsCandidateTableAndSelection(t *testing.T) {
	config := renderer.DefaultConfig()
	config.Debug = true
	r := New(config, WithLayout(LayoutAuto), WithAutoObjective(AutoObjectiveRuntime))
	candidate := &preparedCandidate{options: Options{Layout: LayoutFrames}, metrics: CandidateMetrics{
		FinalBytes: 10, SourceActiveNodes: 11, SourceDefinitionNodes: 12,
		PeakInstantiatedNodes: 13, PeakLiveNodeEstimate: 99, AnimationNodes: 14, LocalViewportCount: 15,
		MaxTranslatedArea: 16,
	}}
	var output bytes.Buffer
	oldWriter, oldFlags := log.Writer(), log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(oldWriter); log.SetFlags(oldFlags) })

	r.logAutoCandidates(candidate, candidate)
	for _, want := range []string{
		"layout=frames", "bytes=10", "source_nodes=11", "definition_nodes=12",
		"peak_instantiated_nodes=13", "animation_nodes=14", "viewport_count=15",
		"viewport_area=0", "translated_area=16", "objective=runtime", "selected=frames",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("debug output %q does not contain %q", output.String(), want)
		}
	}
}

func TestSemanticPlanningHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := buildSemanticPlan(ctx, experimentalRecording(), true, 0, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("buildSemanticPlan() error = %v; want context canceled", err)
	}
}

func TestAutoSerializesSmallestPreparedCandidate(t *testing.T) {
	rec := experimentalRecording()
	config := renderer.DefaultConfig()
	config.Minify = true
	outputs := map[LayoutMode][]byte{}
	metrics := map[LayoutMode]CandidateMetrics{}
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions} {
		r := New(config, WithLayout(layout))
		var out bytes.Buffer
		if err := r.Render(context.Background(), rec, &out); err != nil {
			t.Fatal(err)
		}
		var final bytes.Buffer
		if err := svgoutput.Write(&final, func(w io.Writer) error {
			_, err := w.Write(out.Bytes())
			return err
		}); err != nil {
			t.Fatal(err)
		}
		outputs[layout] = out.Bytes()
		measured, err := r.MeasureCandidate(context.Background(), rec)
		if err != nil {
			t.Fatal(err)
		}
		metrics[layout] = measured
		if want := int64(final.Len()); measured.FinalBytes != want {
			t.Fatalf("%s final bytes = %d; want %d", layout, measured.FinalBytes, want)
		}
	}
	want := LayoutFrames
	if metrics[LayoutBands].FinalBytes < metrics[LayoutFrames].FinalBytes {
		want = LayoutBands
	}
	if metrics[LayoutRegions].FinalBytes < metrics[want].FinalBytes {
		want = LayoutRegions
	}
	var got bytes.Buffer
	if err := New(config, WithLayout(LayoutAuto)).Render(context.Background(), rec, &got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), outputs[want]) {
		t.Fatalf("auto output differs from selected %s candidate", want)
	}
	var explicitSize bytes.Buffer
	if err := New(config, WithLayout(LayoutAuto), WithAutoObjective(AutoObjectiveSize)).Render(context.Background(), rec, &explicitSize); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), explicitSize.Bytes()) {
		t.Fatal("explicit size objective changed default auto output")
	}
}

func TestRuntimeSelectionIsDeterministicForFiniteAndInfiniteLoops(t *testing.T) {
	for _, loops := range []int{0, 3} {
		config := renderer.DefaultConfig()
		config.LoopCount = loops
		r := New(config, WithLayout(LayoutAuto), WithAutoObjective(AutoObjectiveRuntime))
		var selected LayoutMode
		for range 3 {
			plan, err := r.buildSemanticPlan(context.Background(), experimentalRecording())
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := r.prepareSelectedCandidate(context.Background(), experimentalRecording(), &plan)
			if err != nil {
				t.Fatal(err)
			}
			if selected != "" && candidate.options.Layout != selected {
				t.Fatalf("loop count %d selected %q after %q", loops, candidate.options.Layout, selected)
			}
			selected = candidate.options.Layout
		}
	}
}

func TestAutoRenderBuildsSemanticPlanOnce(t *testing.T) {
	r := New(renderer.DefaultConfig(), WithLayout(LayoutAuto))
	var builds atomic.Int64
	r.onSemanticPlanBuild = func() { builds.Add(1) }

	if err := r.Render(context.Background(), experimentalRecording(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("semantic plan builds = %d; want 1", got)
	}
}

func TestAutoRenderSerializesOnlySelectedCandidate(t *testing.T) {
	r := New(renderer.DefaultConfig(), WithLayout(LayoutAuto))
	plans, serializations := 0, 0
	r.onSemanticPlanBuild = func() { plans++ }
	r.onCandidateSerialize = func() { serializations++ }
	if err := r.Render(context.Background(), experimentalRecording(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if plans != 1 || serializations != 1 {
		t.Fatalf("semantic plans/serializations = %d/%d; want 1/1", plans, serializations)
	}
}

func cloneSemanticPlan(source *semanticPlan) semanticPlan {
	plan := *source
	cloneRows := func(rows []ir.Row) []ir.Row {
		out := slices.Clone(rows)
		for i := range out {
			out[i].Runs = slices.Clone(out[i].Runs)
		}
		return out
	}
	plan.staticRows = cloneRows(plan.staticRows)
	plan.content.points = slices.Clone(plan.content.points)
	for i := range plan.content.points {
		plan.content.points[i].state = cloneRows(plan.content.points[i].state)
	}
	plan.cursor.points = slices.Clone(plan.cursor.points)
	plan.usedColors = slices.Clone(plan.usedColors)
	return plan
}
