package svg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestBuildDynamicRegionsKeepsDistantIntervalsSeparate(t *testing.T) {
	plan := renderPlan{duration: time.Second, width: 12, height: 1, content: timeline[[]ir.Row]{
		duration: time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{{Y: 0, Runs: []ir.TextRun{
				{Text: "0", StartCol: 1, EndCol: 2}, {Text: "0", StartCol: 9, EndCol: 10},
			}}}},
			{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{
				{Text: "1", StartCol: 1, EndCol: 2}, {Text: "1", StartCol: 9, EndCol: 10},
			}}}},
		},
	}}

	regions := buildDynamicRegions(&plan, testCatalog())
	if len(regions) != 2 {
		t.Fatalf("regions = %#v; want two distant regions", regions)
	}
	got := [][4]int{
		{regions[0].x, regions[0].y, regions[0].width, regions[0].height},
		{regions[1].x, regions[1].y, regions[1].width, regions[1].height},
	}
	if !reflect.DeepEqual(got, [][4]int{{1, 0, 1, 1}, {9, 0, 1, 1}}) {
		t.Fatalf("region bounds = %#v", got)
	}
}

func TestBuildDynamicRegionsSplitsAdjacentDistinctSchedules(t *testing.T) {
	plan := renderPlan{duration: 3 * time.Second, width: 2, height: 1, content: timeline[[]ir.Row]{
		duration: 3 * time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{{Runs: []ir.TextRun{{Text: "00", EndCol: 2}}}}},
			{time: time.Second, state: []ir.Row{{Runs: []ir.TextRun{{Text: "10", EndCol: 2}}}}},
			{time: 2 * time.Second, state: []ir.Row{{Runs: []ir.TextRun{{Text: "11", EndCol: 2}}}}},
		},
	}}

	got := regionBounds(buildDynamicRegions(&plan, testCatalog()))
	if !reflect.DeepEqual(got, [][4]int{{0, 0, 1, 1}, {1, 0, 1, 1}}) {
		t.Fatalf("region bounds = %#v; want adjacent schedules split", got)
	}
}

func TestBuildDynamicRegionsKeepsAdjacentIdenticalSchedulesTogether(t *testing.T) {
	plan := renderPlan{duration: 2 * time.Second, width: 2, height: 1, content: timeline[[]ir.Row]{
		duration: 2 * time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{{Runs: []ir.TextRun{{Text: "00", EndCol: 2}}}}},
			{time: time.Second, state: []ir.Row{{Runs: []ir.TextRun{{Text: "11", EndCol: 2}}}}},
		},
	}}

	if got := regionBounds(buildDynamicRegions(&plan, testCatalog())); !reflect.DeepEqual(got, [][4]int{{0, 0, 2, 1}}) {
		t.Fatalf("region bounds = %#v; want identical schedules combined", got)
	}
}

func TestBuildDynamicRegionsDoesNotBisectWideGlyphFootprint(t *testing.T) {
	plan := renderPlan{duration: 3 * time.Second, width: 2, height: 1, content: timeline[[]ir.Row]{
		duration: 3 * time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{{Runs: []ir.TextRun{{Text: "界\x00", EndCol: 2}}}}},
			{time: time.Second, state: []ir.Row{{Runs: []ir.TextRun{{Text: "A0", EndCol: 2}}}}},
			{time: 2 * time.Second, state: []ir.Row{{Runs: []ir.TextRun{{Text: "A1", EndCol: 2}}}}},
		},
	}}

	if got := regionBounds(buildDynamicRegions(&plan, testCatalog())); !reflect.DeepEqual(got, [][4]int{{0, 0, 2, 1}}) {
		t.Fatalf("region bounds = %#v; want wide footprint kept atomic", got)
	}
}

func TestOverBudgetSpatialSelectionKeepsVisualAtomsIndivisible(t *testing.T) {
	rows := func(pattern string) []ir.Row {
		out := make([]ir.Row, 0, 21)
		out = append(out, ir.Row{Runs: []ir.TextRun{{Text: "X", EndCol: 1}}})
		for y := 2; y < 22; y++ {
			out = append(out, ir.Row{Y: y, Runs: []ir.TextRun{{Text: pattern, EndCol: 20}}})
		}
		return out
	}
	initial := rows(strings.Repeat("0", 20))
	initial[0].Runs[0] = ir.TextRun{Text: " ", EndCol: 2}
	plan := renderPlan{duration: 3 * time.Second, width: 20, height: 22, content: timeline[[]ir.Row]{
		duration: 3 * time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: initial},
			{time: time.Second, state: rows("10101010101010101010")},
			{time: 2 * time.Second, state: rows(strings.Repeat("1", 20))},
		},
	}}
	rec := parityRecording(plan.width, plan.height, nil)
	c := canvas{
		rec: rec, plan: plan, config: *renderer.DefaultConfig(),
		options: Options{Layout: LayoutRegions}, classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{},
	}
	temporal := buildDynamicRegions(&plan, rec.Colors)
	if pairs := len(mergeableRegionPairs(temporal)); pairs <= regionCandidateEvaluationBudget {
		t.Fatalf("temporal merge pairs = %d; want over budget %d", pairs, regionCandidateEvaluationBudget)
	}

	optimized, err := c.optimizeDynamicRegions(context.Background(), temporal)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(optimized, func(region dynamicRegion) bool {
		return region.x == 0 && region.y == 0 && region.width == 2 && region.height == 1
	}) {
		t.Fatalf("optimized regions bisect the two-cell visual atom: %#v", regionBounds(optimized))
	}
}

func TestOptimizeDynamicRegionsUsesExactBackendProfitability(t *testing.T) {
	plan := renderPlan{duration: time.Second, width: 8, height: 2, content: timeline[[]ir.Row]{
		duration: time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "..", StartCol: 1, EndCol: 3}}},
				{Y: 1, Runs: []ir.TextRun{{Text: "..", StartCol: 3, EndCol: 5}}},
			}},
			{time: time.Second, state: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "##", StartCol: 1, EndCol: 3}}},
				{Y: 1, Runs: []ir.TextRun{{Text: "##", StartCol: 3, EndCol: 5}}},
			}},
		},
	}}

	for _, options := range []Options{
		{Layout: LayoutRegions, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate},
		{Layout: LayoutRegions, Animation: AnimationSMIL, FrameSwitch: FrameSwitchTranslate},
		{Layout: LayoutRegions, Animation: AnimationSMIL, FrameSwitch: FrameSwitchHref},
	} {
		c := canvas{
			rec: parityRecording(8, 2, nil), plan: plan, config: *renderer.DefaultConfig(),
			options: options, classNames: testCatalog().GenerateClassNames(), metrics: &CandidateMetrics{},
		}
		c.rec.Colors = testCatalog()
		regions := buildDynamicRegions(&plan, c.rec.Colors)
		if len(regions) != 2 {
			t.Fatalf("initial regions = %#v; want two smallest intervals", regions)
		}
		separate, err := c.serializedRegionBytes(context.Background(), regions)
		if err != nil {
			t.Fatal(err)
		}
		merged := mergeDynamicRegions(&plan, regions, 0, 1, c.rec.Colors)
		mergedBytes, err := c.serializedRegionBytes(context.Background(), merged)
		if err != nil {
			t.Fatal(err)
		}
		if mergedBytes >= separate {
			t.Fatalf("%+v fixture is not profitable: merged %d, separate %d", options, mergedBytes, separate)
		}
		optimized, err := c.optimizeDynamicRegions(context.Background(), regions)
		if err != nil {
			t.Fatal(err)
		}
		if len(optimized) != 1 || optimized[0].x != 1 || optimized[0].y != 0 ||
			optimized[0].width != 4 || optimized[0].height != 2 {
			t.Fatalf("%+v optimized regions = %#v", options, optimized)
		}
	}
}

func TestRegionMergeDecisionsMatchFullSerializationOracle(t *testing.T) {
	plan := renderPlan{duration: time.Second, width: 12, height: 1, content: timeline[[]ir.Row]{
		duration: time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{{Y: 0, Runs: []ir.TextRun{
				{Text: "a", StartCol: 1, EndCol: 2}, {Text: "b", StartCol: 5, EndCol: 6}, {Text: "c", StartCol: 9, EndCol: 10},
			}}}},
			{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{
				{Text: "A", StartCol: 1, EndCol: 2}, {Text: "B", StartCol: 5, EndCol: 6}, {Text: "C", StartCol: 9, EndCol: 10},
			}}}},
		},
	}}
	for _, minify := range []bool{false, true} {
		for _, options := range []Options{
			{Layout: LayoutRegions, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate},
			{Layout: LayoutRegions, Animation: AnimationSMIL, FrameSwitch: FrameSwitchTranslate},
			{Layout: LayoutRegions, Animation: AnimationSMIL, FrameSwitch: FrameSwitchHref},
		} {
			config := renderer.DefaultConfig()
			config.Minify = minify
			c := canvas{
				rec: parityRecording(12, 1, nil), plan: plan, config: *config, options: options,
				classNames: testCatalog().GenerateClassNames(), metrics: &CandidateMetrics{},
			}
			c.rec.Colors = testCatalog()
			regions := buildDynamicRegions(&plan, c.rec.Colors)
			additive, err := c.optimizeDynamicRegions(context.Background(), regions)
			if err != nil {
				t.Fatal(err)
			}
			oracle, err := optimizeRegionMergesWithBudget(regions, regionCandidateEvaluationBudget,
				func(candidate []dynamicRegion) (int64, error) {
					return c.serializedRegionBytes(context.Background(), candidate)
				}, func(candidate []dynamicRegion, i, j int) []dynamicRegion {
					return mergeDynamicRegions(&plan, candidate, i, j, c.rec.Colors)
				})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(additive, oracle) {
				t.Fatalf("minify=%v options=%+v additive=%#v oracle=%#v", minify, options, additive, oracle)
			}
		}
	}
}

func TestRegionCostRepresentationExcludesUnrelatedSVG(t *testing.T) {
	rec := experimentalRecording()
	config := renderer.DefaultConfig()
	plan, err := buildSemanticPlan(context.Background(), rec, true, 0, config.LoopCount)
	if err != nil {
		t.Fatal(err)
	}
	c := canvas{rec: rec, plan: plan, config: *config, options: Options{
		Layout: LayoutRegions, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate,
	}, classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{}}
	content, err := c.prepareLocalViewports(context.Background(), c.regionBands(buildDynamicRegions(&plan, rec.Colors)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := c.renderRegionRepresentation(context.Background(), &out, &content); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"<defs>", "<svg x=", "@keyframes"} {
		if !strings.Contains(out.String(), required) {
			t.Fatalf("region representation missing %q: %s", required, out.String())
		}
	}
	for _, unrelated := range []string{"clipPath", "static", "cursor", "<circle"} {
		if strings.Contains(out.String(), unrelated) {
			t.Fatalf("region representation contains unrelated %q: %s", unrelated, out.String())
		}
	}
}

func TestOptimizeRegionMergesKeepsTiesSeparate(t *testing.T) {
	regions := []dynamicRegion{{x: 1, width: 1, height: 1}, {x: 2, y: 1, width: 1, height: 1}}
	got, err := optimizeRegionMerges(regions, func([]dynamicRegion) (int64, error) { return 100, nil },
		mergeRegionBounds)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, regions) {
		t.Fatalf("tie merged regions: got %#v, want %#v", got, regions)
	}
}

func TestMergeableRegionPairsIncludesHorizontalAdjacency(t *testing.T) {
	regions := []dynamicRegion{{width: 1, height: 1}, {x: 1, width: 1, height: 1}}
	if got := mergeableRegionPairs(regions); !reflect.DeepEqual(got, []regionMergePair{{i: 0, j: 1}}) {
		t.Fatalf("mergeable pairs = %#v; want horizontal pair", got)
	}
}

func TestOptimizeRegionMergeCostUsesCompleteRegionRepresentation(t *testing.T) {
	regions := []dynamicRegion{
		{x: 1, width: 1, height: 1},
		{x: 1, y: 1, width: 1, height: 1},
		{x: 5, y: 5, width: 1, height: 1},
	}
	got, err := optimizeRegionMerges(regions, func(candidate []dynamicRegion) (int64, error) {
		switch len(candidate) {
		case 3:
			return 100, nil
		case 2:
			if candidate[0].height == 2 || candidate[1].height == 2 {
				return 101, nil
			}
			return 200, nil
		default:
			return 50, nil
		}
	}, mergeRegionBounds)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, regions) {
		t.Fatalf("pair-isolated savings merged a larger complete representation: %#v", got)
	}
}

func TestOptimizeRegionMergesMeasuresUnchangedSetOnce(t *testing.T) {
	regions := []dynamicRegion{
		{x: 1, width: 1, height: 1},
		{x: 1, y: 1, width: 1, height: 1},
		{x: 1, y: 2, width: 1, height: 1},
		{x: 1, y: 3, width: 1, height: 1},
	}
	initialMeasurements := 0
	got, err := optimizeRegionMerges(regions, func(candidate []dynamicRegion) (int64, error) {
		if reflect.DeepEqual(candidate, regions) {
			initialMeasurements++
		}
		return 100, nil
	}, mergeRegionBounds)
	if err != nil {
		t.Fatal(err)
	}
	if initialMeasurements != 1 {
		t.Fatalf("unchanged region set measurements = %d, want 1", initialMeasurements)
	}
	if !reflect.DeepEqual(got, regions) {
		t.Fatalf("tie changed final regions: got %#v, want %#v", got, regions)
	}
}

func TestOptimizeRegionMergesDoesNotPartiallyEvaluateOverBudgetRound(t *testing.T) {
	regions := []dynamicRegion{
		{x: 1, y: 0, width: 1, height: 1},
		{x: 1, y: 1, width: 1, height: 1},
		{x: 1, y: 2, width: 1, height: 1},
		{x: 1, y: 3, width: 1, height: 1},
	}
	measurements := 0
	merges := 0
	got, err := optimizeRegionMergesWithBudget(regions, 3, func(candidate []dynamicRegion) (int64, error) {
		measurements++
		if len(candidate) == 4 {
			return 400, nil
		}
		if len(candidate) == 3 {
			for _, region := range candidate {
				if region.height == 2 {
					return int64(300 + region.y), nil
				}
			}
		}
		return 200, nil
	}, func(candidate []dynamicRegion, i, j int) []dynamicRegion {
		merges++
		return mergeRegionBounds(candidate, i, j)
	})
	if err != nil {
		t.Fatal(err)
	}
	if measurements != 4 {
		t.Fatalf("measurements = %d, want initial set plus three complete-round candidates", measurements)
	}
	if merges != 3 {
		t.Fatalf("merge evaluations = %d, want only three first-round candidates", merges)
	}
	if len(got) != 3 || !slices.ContainsFunc(got, func(region dynamicRegion) bool {
		return region.y == 0 && region.height == 2
	}) {
		t.Fatalf("budgeted regions = %#v, want only the best first-round merge", got)
	}
}

func TestBoundedRegionCostsUseExactCompleteRepresentation(t *testing.T) {
	rec := parityRecording(8, 2, [][]ir.Row{
		{
			parityRow(0, parityRun("..", 1, ir.CellAttrs{})),
			parityRow(1, parityRun("..", 3, ir.CellAttrs{})),
		},
		{
			parityRow(0, parityRun("##", 1, ir.CellAttrs{})),
			parityRow(1, parityRun("##", 3, ir.CellAttrs{})),
		},
	})
	config := renderer.DefaultConfig()
	config.ShowCursor = false
	plan, err := buildSemanticPlan(context.Background(), rec, false, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, minify := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw", true: "minified"}[minify], func(t *testing.T) {
			config.Minify = minify
			c := canvas{
				rec: rec, plan: plan, config: *config,
				options:    Options{Layout: LayoutRegions, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate},
				classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{},
			}
			regions := buildDynamicRegions(&plan, rec.Colors)
			grids := visualGridsForPlan(&plan, rec.Colors)
			type measuredSet struct {
				regions []dynamicRegion
				bytes   int64
			}
			var measured []measuredSet
			_, err := optimizeRegionMergesWithBudget(regions, 1, func(candidate []dynamicRegion) (int64, error) {
				cost, err := c.serializedRegionBytes(context.Background(), candidate)
				if err == nil {
					measured = append(measured, measuredSet{regions: slices.Clone(candidate), bytes: cost})
				}
				return cost, err
			}, func(candidate []dynamicRegion, i, j int) []dynamicRegion {
				return mergeDynamicRegionsFromGrids(&plan, grids, candidate, i, j)
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(measured) != 2 {
				t.Fatalf("measured sets = %d, want current plus one complete candidate", len(measured))
			}
			for _, measurement := range measured {
				assertBoundedRegionCostExact(t, &c, measurement.regions, measurement.bytes, minify)
			}
		})
	}
}

func assertBoundedRegionCostExact(
	t *testing.T,
	c *canvas,
	regions []dynamicRegion,
	measuredBytes int64,
	minify bool,
) {
	t.Helper()
	content, err := c.prepareLocalViewports(context.Background(), c.regionBands(regions))
	if err != nil {
		t.Fatal(err)
	}
	var direct bytes.Buffer
	if err := writeFinalSVG(&direct, minify, func(w io.Writer) error {
		return c.renderRegionRepresentation(context.Background(), w, &content)
	}); err != nil {
		t.Fatal(err)
	}
	if measuredBytes != int64(direct.Len()) {
		t.Fatalf("bounded cost = %d, direct complete representation = %d", measuredBytes, direct.Len())
	}
}

func Test444816BoundedRegionOptimizationMatchesExhaustiveBaseline(t *testing.T) {
	// The pinned constants are the unbounded greedy merge of the spatial
	// fallback set (48 regions, 30 mergeable pairs), which converges after
	// 442 candidate evaluations; the bounded run must reach the same result.
	rec := loadRegionCast(t, "444816.cast")
	config := renderer.DefaultConfig()
	options := Options{Layout: LayoutRegions, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate}
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, 0, config.LoopCount)
	if err != nil {
		t.Fatal(err)
	}
	c := canvas{
		rec: rec, plan: plan, config: *config, options: options,
		classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{},
	}
	regions := buildDynamicRegions(&plan, rec.Colors)
	bounded, err := c.optimizeDynamicRegionsWithBudget(context.Background(), regions, regionCandidateEvaluationBudget)
	if err != nil {
		t.Fatal(err)
	}
	boundedBytes, err := c.serializedRegionBytes(context.Background(), bounded)
	if err != nil {
		t.Fatal(err)
	}
	if len(bounded) != 21 || boundedBytes != 805421 {
		t.Fatalf("bounded result = %d regions/%d bytes, want exhaustive 21/805421", len(bounded), boundedBytes)
	}
	const exhaustiveDigest = "0485ad8f0174d8311450363c58bea2f57e58b2938d3ed5f25b1350e4fafaab1d"
	if digest := regionOptimizationDigest(bounded, boundedBytes); digest != exhaustiveDigest {
		t.Fatalf("bounded result digest = %s, want exhaustive %s", digest, exhaustiveDigest)
	}
	t.Logf("bounded result = %d regions/%d bytes", len(bounded), boundedBytes)
}

func loadRegionCast(t *testing.T, name string) *ir.Recording {
	t.Helper()
	path := filepath.Join("..", "..", "..", "examples", name)
	f, err := os.Open(path) //nolint:gosec // repository test fixture
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cast, err := asciicast.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := ir.NewProcessor(ir.DefaultProcessorConfig()).Process(cast)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func regionBounds(regions []dynamicRegion) [][4]int {
	bounds := make([][4]int, len(regions))
	for i, region := range regions {
		bounds[i] = [4]int{region.x, region.y, region.width, region.height}
	}
	return bounds
}

func regionOptimizationDigest(regions []dynamicRegion, serializedBytes int64) string {
	h := sha256.New()
	writeInt := func(value int64) {
		if err := binary.Write(h, binary.BigEndian, value); err != nil {
			panic(err)
		}
	}
	writeInt(int64(len(regions)))
	for _, bounds := range regionBounds(regions) {
		for _, value := range bounds {
			writeInt(int64(value))
		}
	}
	writeInt(serializedBytes)
	return hex.EncodeToString(h.Sum(nil))
}

func mergeRegionBounds(regions []dynamicRegion, i, j int) []dynamicRegion {
	a, b := regions[i], regions[j]
	merged := dynamicRegion{x: min(a.x, b.x), y: min(a.y, b.y)}
	merged.width = max(a.x+a.width, b.x+b.width) - merged.x
	merged.height = max(a.y+a.height, b.y+b.height) - merged.y
	out := make([]dynamicRegion, 0, len(regions)-1)
	for index := range regions {
		if index != i && index != j {
			out = append(out, regions[index])
		}
	}
	return append(out, merged)
}

func TestOptimizeRegionMergesRejectsBoundingBoxThroughThirdRegion(t *testing.T) {
	plan := renderPlan{duration: time.Second, width: 3, height: 2, content: timeline[[]ir.Row]{
		duration: time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "a", EndCol: 1}, {Text: "c", StartCol: 2, EndCol: 3}}},
				{Y: 1, Runs: []ir.TextRun{{Text: "bbb", EndCol: 3}}},
			}},
			{time: time.Second, state: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "A", EndCol: 1}, {Text: "C", StartCol: 2, EndCol: 3}}},
				{Y: 1, Runs: []ir.TextRun{{Text: "BBB", EndCol: 3}}},
			}},
		},
	}}
	regions := buildDynamicRegions(&plan, testCatalog())
	if len(regions) != 3 {
		t.Fatalf("initial regions = %#v", regions)
	}
	got, err := optimizeRegionMerges(regions, func(candidate []dynamicRegion) (int64, error) {
		return int64(len(candidate) * 100), nil
	}, func(candidate []dynamicRegion, i, j int) []dynamicRegion {
		return mergeDynamicRegions(&plan, candidate, i, j, testCatalog())
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("unsafe merge retained %d regions: %#v", len(got), got)
	}
}

func TestRegionRenderDoesNotDuplicatePaintInsideMergedBounds(t *testing.T) {
	rec := parityRecording(3, 2, [][]ir.Row{
		{
			parityRow(0, parityRun("a", 0, ir.CellAttrs{}), parityRun("c", 2, ir.CellAttrs{})),
			parityRow(1, parityRun("bbb", 0, ir.CellAttrs{})),
		},
		{
			parityRow(0, parityRun("A", 0, ir.CellAttrs{}), parityRun("C", 2, ir.CellAttrs{})),
			parityRow(1, parityRun("BBB", 0, ir.CellAttrs{})),
		},
	})
	config := renderer.DefaultConfig()
	config.ShowCursor = false
	var out bytes.Buffer
	if err := New(config, WithLayout(LayoutRegions)).Render(context.Background(), rec, &out); err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(out.Bytes(), []byte("<svg")); got != 4 {
		t.Fatalf("SVG elements = %d, want root plus three safe regions: %s", got, out.String())
	}
}

func TestBuildDynamicRegionsFallsBackForUnsplittableWideGlyph(t *testing.T) {
	plan := renderPlan{duration: time.Second, width: 4, height: 1, content: timeline[[]ir.Row]{
		duration: time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "ab", StartCol: 0, EndCol: 3}}}}},
			{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "cd", StartCol: 0, EndCol: 3}}}}},
		},
	}}

	regions := buildDynamicRegions(&plan, testCatalog())
	if len(regions) != 1 || len(regions[0].fallbackRows) != 2 || regions[0].width != 4 {
		t.Fatalf("fallback regions = %#v", regions)
	}
}
