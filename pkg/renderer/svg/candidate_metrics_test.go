package svg

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

type candidateWriter struct {
	w            io.Writer
	metrics      *CandidateMetrics
	inDefs       bool
	pendingTag   []byte
	elementStack []int
	nextElement  int
	animatedAt   map[int]bool
}

func (w *candidateWriter) Write(p []byte) (int, error) {
	w.metrics.FinalBytes += int64(len(p))
	w.countElements(p)
	return w.w.Write(p)
}

func (w *candidateWriter) finish() {
	w.metrics.AnimatedElements = len(w.animatedAt)
}

func (w *candidateWriter) countElements(p []byte) {
	if len(w.pendingTag) > 0 {
		p = append(w.pendingTag, p...)
		w.pendingTag = nil
	}
	for len(p) > 0 {
		start := bytes.IndexByte(p, '<')
		if start < 0 {
			return
		}
		p = p[start+1:]
		end := bytes.IndexByte(p, '>')
		if end < 0 {
			w.pendingTag = append(w.pendingTag[:0], '<')
			w.pendingTag = append(w.pendingTag, p...)
			return
		}
		token := bytes.TrimSpace(p[:end])
		p = p[end+1:]
		if len(token) == 0 || token[0] == '!' || token[0] == '?' {
			continue
		}
		if token[0] == '/' {
			w.elementStack = w.elementStack[:len(w.elementStack)-1]
			if string(bytes.TrimSpace(token[1:])) == "defs" {
				w.inDefs = false
			}
			continue
		}
		name := token
		if space := bytes.IndexAny(name, " \t\r\n/"); space >= 0 {
			name = name[:space]
		}
		nameString := string(name)
		w.countElement(nameString)
		if bytes.Contains(token, []byte(`style="animation:`)) ||
			(nameString == "rect" && bytes.Contains(token, []byte(`class="cursor"`))) {
			w.markAnimated(w.nextElement + 1)
		}
		if nameString == "animate" || nameString == "animateTransform" || nameString == "animateMotion" {
			if len(w.elementStack) > 0 {
				w.markAnimated(w.elementStack[len(w.elementStack)-1])
			}
		}
		if string(name) == "defs" {
			w.inDefs = true
		}
		if !bytes.HasSuffix(token, []byte{'/'}) {
			w.nextElement++
			w.elementStack = append(w.elementStack, w.nextElement)
		}
	}
}

func (w *candidateWriter) markAnimated(element int) {
	if w.animatedAt == nil {
		w.animatedAt = map[int]bool{}
	}
	w.animatedAt[element] = true
}

func (w *candidateWriter) countElement(name string) {
	w.metrics.XMLNodes++
	if w.inDefs || name == "defs" {
		w.metrics.DefinitionNodes++
	} else {
		w.metrics.ActiveNodes++
	}
	switch name {
	case "text":
		w.metrics.TextNodes++
	case "rect":
		w.metrics.RectNodes++
	case "g":
		w.metrics.GroupNodes++
	case "use":
		w.metrics.UseNodes++
	case "animate", "animateTransform", "animateMotion":
		w.metrics.AnimationNodes++
	}
}

func TestMeasureCandidateReportsSerializedStructure(t *testing.T) {
	r := New(renderer.DefaultConfig(), WithAnimation(AnimationSMIL))
	metrics, err := r.MeasureCandidate(context.Background(), experimentalRecording())
	if err != nil {
		t.Fatal(err)
	}
	if metrics.FinalBytes == 0 || metrics.XMLNodes == 0 || metrics.TextNodes == 0 ||
		metrics.GroupNodes == 0 || metrics.AnimationNodes == 0 || metrics.AnimatedElements == 0 {
		t.Fatalf("incomplete candidate metrics: %#v", metrics)
	}
	if metrics.XMLNodes != metrics.DefinitionNodes+metrics.ActiveNodes {
		t.Fatalf("nodes = %d, definitions %d + active %d", metrics.XMLNodes,
			metrics.DefinitionNodes, metrics.ActiveNodes)
	}
	if metrics.MaxTranslatedWidth <= 0 || metrics.MaxTranslatedArea <= 0 {
		t.Fatalf("translated surface metrics: %#v", metrics)
	}
}

func TestCSSMetricsCountAnimatedContentAndCursorElements(t *testing.T) {
	metrics, err := New(renderer.DefaultConfig()).MeasureCandidate(context.Background(), experimentalRecording())
	if err != nil {
		t.Fatal(err)
	}
	if metrics.AnimationNodes != 0 {
		t.Fatalf("CSS animation nodes = %d, want 0", metrics.AnimationNodes)
	}
	if metrics.AnimatedElements != 3 {
		t.Fatalf("CSS animated elements = %d, want content parent, cursor parent, and blinking cursor",
			metrics.AnimatedElements)
	}
}

func TestRegionMetricsUseNarrowLocalViewports(t *testing.T) {
	rec := parityRecording(120, 40, [][]ir.Row{
		{{Y: 20, Runs: []ir.TextRun{{Text: "0", StartCol: 60, EndCol: 61}}}},
		{{Y: 20, Runs: []ir.TextRun{{Text: "1", StartCol: 60, EndCol: 61}}}},
	})
	metrics, err := New(renderer.DefaultConfig(), WithLayout(LayoutRegions)).MeasureCandidate(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.LocalViewportCount != 1 || metrics.MaxViewportWidth >= rec.Width*ColWidth ||
		metrics.MaxTranslatedArea >= int64(rec.Width*ColWidth*rec.Height*RowHeight) {
		t.Fatalf("region surface metrics = %#v", metrics)
	}
}

func TestViewportAreaUsesLargestEmittedViewport(t *testing.T) {
	states := [][]ir.Row{
		append([]ir.Row{parityRow(0, parityRun("0000000000", 0, ir.CellAttrs{}))}, narrowRows("0")...),
		append([]ir.Row{parityRow(0, parityRun("1111111111", 0, ir.CellAttrs{}))}, narrowRows("1")...),
	}
	metrics, err := New(renderer.DefaultConfig(), WithLayout(LayoutBands)).MeasureCandidate(
		context.Background(), parityRecording(20, 20, states))
	if err != nil {
		t.Fatal(err)
	}
	if metrics.MaxViewportWidth != 132 || metrics.MaxViewportHeight != 250 || metrics.MaxViewportArea != 9000 {
		t.Fatalf("viewport maxima = %dx%d, area %d; want 132x250, area 9000",
			metrics.MaxViewportWidth, metrics.MaxViewportHeight, metrics.MaxViewportArea)
	}
}

func narrowRows(value string) []ir.Row {
	rows := make([]ir.Row, 10)
	for i := range rows {
		rows[i] = parityRow(i+10, parityRun(value, 15, ir.CellAttrs{}))
	}
	return rows
}

func TestSparseRegionsTransformLessAreaThanFramesAndBands(t *testing.T) {
	rec := regionBenchmarkRecordings()["120x40_four_distant_counters"]
	metrics := map[LayoutMode]CandidateMetrics{}
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions} {
		measured, err := New(renderer.DefaultConfig(), WithLayout(layout)).MeasureCandidate(context.Background(), rec)
		if err != nil {
			t.Fatal(err)
		}
		metrics[layout] = measured
	}
	regions := metrics[LayoutRegions]
	if regions.MaxViewportWidth >= metrics[LayoutBands].MaxViewportWidth ||
		regions.MaxTranslatedArea >= metrics[LayoutBands].MaxTranslatedArea ||
		regions.MaxTranslatedArea >= metrics[LayoutFrames].MaxTranslatedArea {
		t.Fatalf("regions = %#v, bands = %#v, frames = %#v", regions, metrics[LayoutBands], metrics[LayoutFrames])
	}
}

func TestPreparedMetricsMatchSerializedStructure(t *testing.T) {
	for _, variant := range parityOptions {
		t.Run(variant.name, func(t *testing.T) {
			rec := experimentalRecording()
			config := renderer.DefaultConfig()
			options := DefaultOptions()
			for _, apply := range variant.options {
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
			parsed := CandidateMetrics{}
			writer := &candidateWriter{w: io.Discard, metrics: &parsed}
			r := New(config, variant.options...)
			if err := r.serializeCandidate(context.Background(), rec, writer, candidate); err != nil {
				t.Fatal(err)
			}
			writer.finish()

			got := candidate.metrics
			got.FinalBytes = 0
			got.StateDefinitions = 0
			got.MaxUseDepth = 0
			got.MaxTranslatedWidth = 0
			got.MaxTranslatedArea = 0
			got.LocalViewportCount = 0
			got.MaxViewportWidth = 0
			got.MaxViewportHeight = 0
			got.MaxViewportArea = 0
			got.SourceActiveNodes = 0
			got.SourceDefinitionNodes = 0
			got.StaticUseShadowNodes = 0
			got.InitialAnimatedUseShadowNodes = 0
			got.PeakAnimatedUseShadowNodes = 0
			got.PeakInstantiatedNodes = 0
			got.DurationWeightedInstantiatedNodeNanos = 0
			got.UseTargetSwitches = 0
			got.PeakLiveNodeEstimate = 0
			parsed.FinalBytes = 0
			if !reflect.DeepEqual(got, parsed) {
				t.Fatalf("prepared metrics = %#v; serialized = %#v", got, parsed)
			}
		})
	}
}

func TestExistingChildIDRetainsWrapperInExactMetrics(t *testing.T) {
	row := &renderedRow{row: ir.Row{Runs: []ir.TextRun{{Text: "x", EndCol: 1}}}, svg: `<text id="row">x</text>`}
	content := preparedContent{
		frameStateIDs:  []string{"state"},
		frameRows:      [][]*renderedRow{{row}},
		frameKeyframes: []keyframePoint[int]{{selector: "0%", state: 0}, {selector: "100%", state: 0}},
	}
	metrics := CandidateMetrics{}
	c := canvas{
		rec:     parityRecording(1, 1, nil),
		plan:    renderPlan{duration: time.Second},
		options: Options{Layout: LayoutFrames, Animation: AnimationSMIL, FrameSwitch: FrameSwitchHref},
	}
	if err := addStructuralMetrics(&metrics, &c, &content); err != nil {
		t.Fatal(err)
	}
	if metrics.XMLNodes != 11 || metrics.DefinitionNodes != 5 || metrics.ActiveNodes != 6 ||
		metrics.GroupNodes != 2 || metrics.SourceActiveNodes != 6 || metrics.SourceDefinitionNodes != 5 ||
		metrics.InitialAnimatedUseShadowNodes != 2 || metrics.PeakAnimatedUseShadowNodes != 2 ||
		metrics.PeakInstantiatedNodes != 13 || metrics.DurationWeightedInstantiatedNodeNanos != 13_000_000_000 ||
		metrics.PeakLiveNodeEstimate != 8 || metrics.MaxUseDepth != 1 {
		t.Fatalf("collision fallback metrics = %#v", metrics)
	}
}

func TestCandidateWriterCountsTagsSplitAcrossWrites(t *testing.T) {
	metrics := CandidateMetrics{}
	w := candidateWriter{w: io.Discard, metrics: &metrics}
	for _, part := range [][]byte{[]byte("<svg><de"), []byte("fs><g/>"), []byte("</defs><text>x</text></svg>")} {
		if _, err := w.Write(part); err != nil {
			t.Fatal(err)
		}
	}
	if metrics.XMLNodes != 4 || metrics.DefinitionNodes != 2 || metrics.ActiveNodes != 2 ||
		metrics.GroupNodes != 1 || metrics.TextNodes != 1 {
		t.Fatalf("split metrics = %#v", metrics)
	}
	if metrics.FinalBytes != int64(len("<svg><defs><g/></defs><text>x</text></svg>")) {
		t.Fatalf("bytes = %d", metrics.FinalBytes)
	}
}

func TestCandidateWriterCountsAnimatedElementsOnce(t *testing.T) {
	metrics := CandidateMetrics{}
	w := candidateWriter{w: io.Discard, metrics: &metrics}
	_, _ = w.Write([]byte(`<svg><g><animate/><animateTransform/></g></svg>`))
	w.finish()
	if metrics.AnimationNodes != 2 || metrics.AnimatedElements != 1 {
		t.Fatalf("animation metrics = %#v", metrics)
	}
}

func TestCandidateWriterDistinguishesAnimatedSiblings(t *testing.T) {
	metrics := CandidateMetrics{}
	w := candidateWriter{w: io.Discard, metrics: &metrics}
	_, _ = w.Write([]byte(`<svg><g><animate/></g><g><animate/></g></svg>`))
	w.finish()
	if metrics.AnimatedElements != 2 {
		t.Fatalf("animated elements = %d", metrics.AnimatedElements)
	}
}
