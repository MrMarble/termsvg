package svg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestCandidateCostMatchesSerialization(t *testing.T) {
	recordings := map[string]func() *ir.Recording{
		"small":        createTestRecording,
		"experimental": experimentalRecording,
	}
	configs := []struct {
		name                   string
		minify, window, cursor bool
		loops                  int
	}{
		{name: "raw-borderless-no-cursor-infinite", loops: 0},
		{name: "minified-window-cursor-finite", minify: true, window: true, cursor: true, loops: 3},
		{name: "raw-window-cursor-no-loop", window: true, cursor: true, loops: -1},
		{name: "minified-borderless-cursor-infinite", minify: true, cursor: true, loops: 0},
	}
	validModes := []struct {
		animation AnimationMode
		switcher  FrameSwitchMode
	}{
		{AnimationCSS, FrameSwitchTranslate},
		{AnimationSMIL, FrameSwitchTranslate},
		{AnimationSMIL, FrameSwitchHref},
	}
	for name, load := range recordings {
		rec := load()
		for _, testConfig := range configs {
			t.Run(name+"/"+testConfig.name, func(t *testing.T) {
				config := renderer.DefaultConfig()
				config.Minify, config.ShowWindow, config.ShowCursor, config.LoopCount =
					testConfig.minify, testConfig.window, testConfig.cursor, testConfig.loops
				for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions} {
					for _, mode := range validModes {
						for _, fps := range []int{0, 30} {
							options := Options{Layout: layout, Animation: mode.animation, FrameSwitch: mode.switcher, MaxFPS: fps}
							t.Run(fmt.Sprintf("%s-%s-%s-%dfps", layout, mode.animation, mode.switcher, fps), func(t *testing.T) {
								assertCandidateCostExact(t, rec, config, &options)
							})
						}
					}
				}
			})
		}
	}
}

func TestCandidateCostMatchesRandomizedRecordings(t *testing.T) {
	rng := rand.New(rand.NewSource(0x434f5354)) //nolint:gosec // deterministic fixtures
	for i := range 10 {
		rec := randomParityRecording(rng)
		config := renderer.DefaultConfig()
		config.Minify = i%2 == 0
		config.ShowWindow = i%3 == 0
		config.ShowCursor = i%4 != 0
		config.LoopCount = []int{0, -1, 3}[i%3]
		options := DefaultOptions()
		options.Layout = []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions}[i%3]
		options.Animation = []AnimationMode{AnimationCSS, AnimationSMIL}[i%2]
		options.FrameSwitch = []FrameSwitchMode{FrameSwitchTranslate, FrameSwitchHref}[i%2]
		if i%2 == 1 {
			options.MaxFPS = 30
		}
		assertCandidateCostExact(t, rec, config, &options)
	}
}

func TestCandidateCostMatchesRealCasts(t *testing.T) {
	modes := []Options{
		{Layout: LayoutFrames, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate},
		{Layout: LayoutBands, Animation: AnimationSMIL, FrameSwitch: FrameSwitchHref},
		{Layout: LayoutRegions, Animation: AnimationSMIL, FrameSwitch: FrameSwitchTranslate},
		{Layout: LayoutFrames, Animation: AnimationSMIL, FrameSwitch: FrameSwitchHref},
		{Layout: LayoutRegions, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate},
	}
	for i, name := range []string{"256colors.cast", "444816.cast", "htop.cast", "session.cast", "rgb.cast"} {
		rec := loadRegionCast(t, name)
		config := renderer.DefaultConfig()
		config.Minify = i%2 == 0
		config.ShowWindow = i%3 != 0
		config.ShowCursor = i%2 != 0
		assertCandidateCostExact(t, rec, config, &modes[i])
	}
}

func assertCandidateCostExact(t *testing.T, rec *ir.Recording, config *renderer.Config, options *Options) {
	t.Helper()
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, options.MaxFPS, config.LoopCount)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := prepareCandidate(context.Background(), rec, &plan, config, options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := costPreparedCandidate(context.Background(), rec, config, candidate)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	r := &Renderer{config: *config}
	serialize := func(w io.Writer) error { return r.serializeCandidate(context.Background(), rec, w, candidate) }
	if err := writeFinalSVG(&out, config.Minify, serialize); err != nil {
		t.Fatal(err)
	}
	if got != int64(out.Len()) {
		if config.Minify {
			var raw bytes.Buffer
			if err := r.serializeCandidate(context.Background(), rec, &raw, candidate); err != nil {
				t.Fatal(err)
			}
			normalized := bytes.ReplaceAll(raw.Bytes(), []byte("\u00a0"), []byte(" "))
			for i := 0; i < min(len(normalized), out.Len()); i++ {
				if normalized[i] != out.Bytes()[i] {
					t.Logf("first diff raw=%q min=%q",
						normalized[max(0, i-40):min(len(normalized), i+80)], out.Bytes()[max(0, i-40):min(out.Len(), i+80)])
					break
				}
			}
		}
		t.Fatalf("cost = %d, serialization = %d", got, out.Len())
	}
}

func TestRecursiveDefinitionCostRejectsCycles(t *testing.T) {
	graph := map[string]definitionNode{"a": {nodes: 1, uses: []string{"b"}}, "b": {nodes: 1, uses: []string{"a"}}}
	if _, _, err := recursiveDefinitionCost(graph, "a", map[string]bool{}); err == nil {
		t.Fatal("cyclic use graph was accepted")
	}
}

func TestUseMetricArithmeticSaturates(t *testing.T) {
	if got := saturatingAdd(math.MaxUint64, 1); got != math.MaxUint64 {
		t.Fatalf("saturatingAdd = %d", got)
	}
	if got := saturatingMul(math.MaxUint64, 2); got != math.MaxUint64 {
		t.Fatalf("saturatingMul = %d", got)
	}
}

func TestCandidateComparisonDoesNotWalkSerializationFragments(t *testing.T) {
	rec := experimentalRecording()
	config := renderer.DefaultConfig()
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, 0, config.LoopCount)
	if err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions()
	candidate, err := prepareCandidate(context.Background(), rec, &plan, config, &options)
	if err != nil {
		t.Fatal(err)
	}
	walks := candidate.cost.fragmentWalks
	if _, err := costPreparedCandidate(context.Background(), rec, config, candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.cost.fragmentWalks != walks {
		t.Fatalf("comparison fragment walks = %d; want cached %d", candidate.cost.fragmentWalks, walks)
	}
	contentWalks := candidate.content.cost.fragmentWalks
	if _, err := costPreparedContent(context.Background(), &canvas{}, &candidate.content); err != nil {
		t.Fatal(err)
	}
	if candidate.content.cost.fragmentWalks != contentWalks {
		t.Fatalf("region comparison fragment walks = %d; want cached %d", candidate.content.cost.fragmentWalks, contentWalks)
	}
}

func TestUseExpansionMetricsValues(t *testing.T) {
	small := &renderedRow{id: "r0", svg: `<text y="20">x</text>`, row: ir.Row{Runs: []ir.TextRun{{Text: "x", EndCol: 1}}}}
	large := &renderedRow{
		id:  "r1",
		svg: `<text y="20">x</text><text x="12" y="20">y</text>`,
		row: ir.Row{Runs: []ir.TextRun{{Text: "x", EndCol: 1}, {Text: "y", StartCol: 1, EndCol: 2}}},
	}
	content := preparedContent{rowDefs: []*renderedRow{small, large}, bands: []preparedBand{
		{
			stateIDs: []string{"a0", "a1"},
			rows:     [][]*renderedRow{{small}, {large}},
			keyframes: []keyframePoint[int]{
				{selector: "0%", state: 0}, {selector: "25%", state: 1}, {selector: "100%", state: 1},
			},
		},
		{
			stateIDs: []string{"b0", "b1"},
			rows:     [][]*renderedRow{{large}, {small}},
			keyframes: []keyframePoint[int]{
				{selector: "0%", state: 0}, {selector: "75%", state: 1}, {selector: "100%", state: 1},
			},
		},
	}}
	metrics := CandidateMetrics{XMLNodes: 10, ActiveNodes: 5, DefinitionNodes: 5}
	c := canvas{
		rec:     parityRecording(2, 1, nil),
		plan:    renderPlan{duration: time.Second},
		options: Options{Layout: LayoutBands, FrameSwitch: FrameSwitchHref},
	}
	if err := addUseExpansionMetrics(&metrics, &c, &content); err != nil {
		t.Fatal(err)
	}
	if metrics.SourceActiveNodes != 5 || metrics.SourceDefinitionNodes != 5 || metrics.StaticUseShadowNodes != 8 ||
		metrics.InitialAnimatedUseShadowNodes != 6 || metrics.PeakAnimatedUseShadowNodes != 8 ||
		metrics.PeakInstantiatedNodes != 26 || metrics.DurationWeightedInstantiatedNodeNanos != 25_000_000_000 ||
		metrics.UseTargetSwitches != 2 || metrics.PeakLiveNodeEstimate != 13 || metrics.MaxUseDepth != 2 {
		t.Fatalf("use metrics = %#v", metrics)
	}
}

func TestUseExpansionMetricsCountRetainedStateWrappers(t *testing.T) {
	inline := func(text string) *renderedRow {
		return &renderedRow{row: ir.Row{Runs: []ir.TextRun{{Text: text, EndCol: 1}}}}
	}
	rowDef := inline("r")
	rowDef.id = "r"
	states := [][]*renderedRow{{inline("a"), inline("b")}, {inline("c"), rowDef}}
	keyframes := []keyframePoint[int]{{selector: "0%", state: 0}, {selector: "50%", state: 1}}

	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions} {
		t.Run(string(layout), func(t *testing.T) {
			content := preparedContent{rowDefs: []*renderedRow{rowDef}}
			if layout == LayoutFrames {
				content.frameStateIDs, content.frameRows, content.frameKeyframes = []string{"s0", "s1"}, states, keyframes
			} else {
				content.bands = []preparedBand{{stateIDs: []string{"s0", "s1"}, rows: states, keyframes: keyframes}}
			}
			metrics := CandidateMetrics{XMLNodes: 10, ActiveNodes: 4, DefinitionNodes: 6}
			c := canvas{
				rec:     parityRecording(2, 1, nil),
				plan:    renderPlan{duration: time.Second},
				options: Options{Layout: layout, FrameSwitch: FrameSwitchHref},
			}
			if err := addUseExpansionMetrics(&metrics, &c, &content); err != nil {
				t.Fatal(err)
			}
			if metrics.SourceActiveNodes != 4 || metrics.SourceDefinitionNodes != 6 ||
				metrics.StaticUseShadowNodes != 1 || metrics.InitialAnimatedUseShadowNodes != 3 ||
				metrics.PeakAnimatedUseShadowNodes != 4 || metrics.PeakInstantiatedNodes != 15 ||
				metrics.DurationWeightedInstantiatedNodeNanos != 14_500_000_000 ||
				metrics.PeakLiveNodeEstimate != 8 || metrics.MaxUseDepth != 2 {
				t.Fatalf("%s use metrics = %#v", layout, metrics)
			}
		})
	}
}
