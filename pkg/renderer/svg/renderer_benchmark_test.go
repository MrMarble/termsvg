package svg

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func BenchmarkRender(b *testing.B) {
	b.Run("small_two_frame", func(b *testing.B) { benchmarkRecording(b, createTestRecording()) })
	b.Run("many_frames_same_static_rows", func(b *testing.B) { benchmarkRecording(b, staticFrames(200)) })
	b.Run("cursor_only_changes", func(b *testing.B) { benchmarkRecording(b, cursorFrames(200)) })
	b.Run("color_background_heavy", func(b *testing.B) { benchmarkRecording(b, colorFrames()) })
	b.Run("repeated_dynamic_rows", func(b *testing.B) { benchmarkRecording(b, repeatedRows(200)) })
	benchmarkCast(b, "256colors.cast")
	benchmarkCast(b, "444816.cast")
}

func BenchmarkCandidateMatrix(b *testing.B) {
	fixtures := []struct {
		name       string
		borderless bool
	}{
		{name: "444816.cast"},
		{name: "444816.cast", borderless: true},
		{name: "htop.cast"},
		{name: "session.cast"},
		{name: "256colors.cast"},
		{name: "rgb.cast"},
	}
	for _, fixture := range fixtures {
		benchmarkCastMatrix(b, fixture.name, fixture.borderless)
	}
}

func BenchmarkSemanticPlanConstruction(b *testing.B) {
	rec := staticFrames(200)
	b.ReportAllocs()
	for range b.N {
		if _, err := buildSemanticPlan(context.Background(), rec, true, 0, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFramePreparation(b *testing.B) {
	benchmarkPreparation(b, LayoutFrames)
}

func BenchmarkBandPreparation(b *testing.B) {
	benchmarkPreparation(b, LayoutBands)
}

func BenchmarkRegionPreparation(b *testing.B) {
	benchmarkPreparation(b, LayoutRegions)
}

func BenchmarkRegionWorkloads(b *testing.B) {
	for name, rec := range regionBenchmarkRecordings() {
		b.Run(name, func(b *testing.B) {
			benchmarkCandidate(b, rec, renderer.DefaultConfig(), WithLayout(LayoutRegions))
		})
	}
}

func BenchmarkScrollTape(b *testing.B) {
	fixtures := map[string]*ir.Recording{"synthetic-120x40": scrollRecording(120, 40, 21)}
	for _, name := range []string{"htop.cast", "session.cast"} {
		path := filepath.Join("..", "..", "..", "examples", name)
		f, err := os.Open(path) //nolint:gosec // repository benchmark fixture
		if err != nil {
			continue
		}
		cast, parseErr := asciicast.Parse(f)
		closeErr := f.Close()
		if parseErr != nil {
			b.Fatal(parseErr)
		}
		if closeErr != nil {
			b.Fatal(closeErr)
		}
		rec, err := ir.NewProcessor(ir.DefaultProcessorConfig()).Process(cast)
		if err != nil {
			b.Fatal(err)
		}
		fixtures[strings.TrimSuffix(name, ".cast")] = rec
	}
	for name, rec := range fixtures {
		for _, layout := range []LayoutMode{LayoutBands, LayoutScroll} {
			b.Run(name+"/"+string(layout), func(b *testing.B) {
				benchmarkScrollCandidate(b, rec, WithLayout(layout))
			})
		}
	}
}

func BenchmarkRetainedBackgroundTracks(b *testing.B) {
	rec := rectTrackRecording(b, func() []rectState {
		states := make([]rectState, 64)
		for i := 1; i < len(states)-1; i++ {
			states[i] = rectState{x: i % 3, width: i%24 + 1}
		}
		return states
	}(), false)
	for _, primitives := range []PrimitiveMode{PrimitiveSnapshots, PrimitiveRectTracks} {
		b.Run(string(primitives), func(b *testing.B) {
			benchmarkScrollCandidate(b, rec,
				WithLayout(LayoutRegions), WithAnimation(AnimationSMIL), WithPrimitiveMode(primitives))
		})
	}
}

func benchmarkScrollCandidate(b *testing.B, rec *ir.Recording, options ...Option) {
	b.Helper()
	r := New(renderer.DefaultConfig(), options...)
	metrics, err := r.MeasureCandidate(context.Background(), rec)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := r.MeasureCandidate(context.Background(), rec); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(metrics.FinalBytes), "raw-bytes")
	b.ReportMetric(float64(metrics.XMLNodes), "xml-nodes")
	b.ReportMetric(float64(metrics.DefinitionNodes), "definitions")
	b.ReportMetric(float64(metrics.AnimatedElements), "animated-elements")
	b.ReportMetric(float64(metrics.LocalViewportCount), "viewports")
	b.ReportMetric(float64(metrics.MaxTapeArea), "tape-area")
	b.ReportMetric(float64(metrics.MaxTranslatedArea), "translated-area")
	b.ReportMetric(float64(metrics.RetainedPrimitiveCount), "retained-primitives")
	b.ReportMetric(float64(metrics.GeometryAnimationNodes), "geometry-animations")
	b.ReportMetric(float64(metrics.PaintPropertyAnimationNodes), "paint-animations")
}

func BenchmarkAutoSelection(b *testing.B) {
	rec := staticFrames(200)
	config := renderer.DefaultConfig()
	r := New(config, WithLayout(LayoutAuto))
	b.ReportAllocs()
	for range b.N {
		if _, err := r.MeasureCandidate(context.Background(), rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStylePlanSelection(b *testing.B) {
	rec := colorFrames()
	for _, style := range []StyleMode{StyleLegacy, StyleAuto} {
		for _, minify := range []bool{false, true} {
			name := "raw"
			if minify {
				name = "minified"
			}
			b.Run(string(style)+"/"+name, func(b *testing.B) {
				config := renderer.DefaultConfig()
				config.Minify = minify
				r := New(config, WithStyleMode(style))
				metrics, err := r.MeasureCandidate(context.Background(), rec)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, err := r.MeasureCandidate(context.Background(), rec); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(metrics.FinalBytes), "svg-bytes")
			})
		}
	}
}

func BenchmarkSelectedSerialization(b *testing.B) {
	rec := staticFrames(200)
	config := renderer.DefaultConfig()
	r := New(config, WithLayout(LayoutAuto))
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, 0, config.LoopCount)
	if err != nil {
		b.Fatal(err)
	}
	candidate, err := r.prepareSelectedCandidate(context.Background(), rec, &plan)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := r.serializeCandidate(context.Background(), rec, io.Discard, candidate); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkPreparation(b *testing.B, layout LayoutMode) {
	b.Helper()
	rec := staticFrames(200)
	config := renderer.DefaultConfig()
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, 0, config.LoopCount)
	if err != nil {
		b.Fatal(err)
	}
	options := DefaultOptions()
	options.Layout = layout
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := prepareCandidate(context.Background(), rec, &plan, config, &options); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkRecording(b *testing.B, rec *ir.Recording) {
	b.Helper()
	r := New(renderer.DefaultConfig())
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := r.Render(ctx, rec, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkCast(b *testing.B, name string) {
	b.Helper()
	b.Run(name, func(b *testing.B) {
		path := filepath.Join("..", "..", "..", "examples", name)
		f, err := os.Open(path) //nolint:gosec // repository benchmark fixture
		if os.IsNotExist(err) {
			b.Skipf("optional fixture not found: %s", path)
		}
		if err != nil {
			b.Fatal(err)
		}
		cast, err := asciicast.Parse(f)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			b.Fatal(err)
		}
		rec, err := ir.NewProcessor(ir.DefaultProcessorConfig()).Process(cast)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkRecording(b, rec)
	})
}

func benchmarkCastMatrix(b *testing.B, name string, borderless bool) {
	b.Helper()
	path := filepath.Join("..", "..", "..", "examples", name)
	f, err := os.Open(path) //nolint:gosec // repository benchmark fixture
	if os.IsNotExist(err) {
		b.Skipf("optional fixture not found: %s", path)
	}
	if err != nil {
		b.Fatal(err)
	}
	cast, err := asciicast.Parse(f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		b.Fatal(err)
	}
	rec, err := ir.NewProcessor(ir.DefaultProcessorConfig()).Process(cast)
	if err != nil {
		b.Fatal(err)
	}
	fixture := strings.TrimSuffix(name, filepath.Ext(name))
	if borderless {
		fixture += "-borderless"
	}
	for _, fps := range []int{0, 30} {
		for _, variant := range parityOptions {
			options := append(slices.Clone(variant.options), WithMaxFPS(fps))
			label := fmt.Sprintf("%s/%dfps/%s", fixture, fps, variant.name)
			b.Run(label, func(b *testing.B) {
				config := renderer.DefaultConfig()
				config.ShowWindow = !borderless
				benchmarkCandidate(b, rec, config, options...)
			})
		}
	}
}

func benchmarkCandidate(b *testing.B, rec *ir.Recording, config *renderer.Config, options ...Option) {
	b.Helper()
	r := New(config, options...)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := r.MeasureCandidate(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}

func staticFrames(count int) *ir.Recording {
	rec := createTestRecording()
	rows := []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "unchanged output"}}}}
	rec.Frames = make([]ir.Frame, count)
	for i := range rec.Frames {
		rec.Frames[i] = ir.Frame{Time: time.Duration(i) * time.Millisecond, Index: i, Rows: rows}
	}
	rec.Duration = time.Duration(count) * time.Millisecond
	return rec
}

func cursorFrames(count int) *ir.Recording {
	rec := staticFrames(count)
	for i := range rec.Frames {
		rec.Frames[i].Cursor = ir.Cursor{Col: i % rec.Width, Row: i % rec.Height, Visible: true}
	}
	return rec
}

func colorFrames() *ir.Recording {
	rec := createTestRecording()
	palette := termcolor.Standard()
	rows := make([]ir.Row, 16)
	for i := range rows {
		fg := rec.Colors.Register(termcolor.FromRGB(uint8(i*15), uint8(255-i*12), uint8(i*7)), &palette)
		bg := rec.Colors.Register(termcolor.FromRGB(uint8(255-i*9), uint8(i*13), uint8(128+i)), &palette)
		rows[i] = ir.Row{Y: i, Runs: []ir.TextRun{{
			Text: fmt.Sprintf("color %02d", i), EndCol: 8, Attrs: ir.CellAttrs{FG: fg, BG: bg},
		}}}
	}
	rec.Frames = []ir.Frame{{Rows: rows}}
	rec.Duration = time.Second
	rec.Stats.HasTrueColor = true
	return rec
}

func repeatedRows(count int) *ir.Recording {
	rec := createTestRecording()
	variants := [4][]ir.Row{}
	for i := range variants {
		variants[i] = []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: fmt.Sprintf("dynamic row %d", i)}}}}
	}
	rec.Frames = make([]ir.Frame, count)
	for i := range rec.Frames {
		rec.Frames[i] = ir.Frame{Time: time.Duration(i) * time.Millisecond, Index: i, Rows: variants[i%len(variants)]}
	}
	rec.Duration = time.Duration(count) * time.Millisecond
	return rec
}

func regionBenchmarkRecordings() map[string]*ir.Recording {
	fixtures := tuiParityFixtures()
	return map[string]*ir.Recording{
		"120x40_one_counter": parityRecording(120, 40, [][]ir.Row{
			{parityRow(20, parityRun("0", 60, ir.CellAttrs{}))},
			{parityRow(20, parityRun("1", 60, ir.CellAttrs{}))},
		}),
		"120x40_four_distant_counters": parityRecording(120, 40, [][]ir.Row{
			{
				parityRow(5, parityRun("0", 5, ir.CellAttrs{}), parityRun("0", 110, ir.CellAttrs{})),
				parityRow(35, parityRun("0", 5, ir.CellAttrs{}), parityRun("0", 110, ir.CellAttrs{})),
			},
			{
				parityRow(5, parityRun("1", 5, ir.CellAttrs{}), parityRun("0", 110, ir.CellAttrs{})),
				parityRow(35, parityRun("0", 5, ir.CellAttrs{}), parityRun("0", 110, ir.CellAttrs{})),
			},
			{
				parityRow(5, parityRun("1", 5, ir.CellAttrs{}), parityRun("1", 110, ir.CellAttrs{})),
				parityRow(35, parityRun("0", 5, ir.CellAttrs{}), parityRun("0", 110, ir.CellAttrs{})),
			},
			{
				parityRow(5, parityRun("1", 5, ir.CellAttrs{}), parityRun("1", 110, ir.CellAttrs{})),
				parityRow(35, parityRun("1", 5, ir.CellAttrs{}), parityRun("0", 110, ir.CellAttrs{})),
			},
			{
				parityRow(5, parityRun("1", 5, ir.CellAttrs{}), parityRun("1", 110, ir.CellAttrs{})),
				parityRow(35, parityRun("1", 5, ir.CellAttrs{}), parityRun("1", 110, ir.CellAttrs{})),
			},
		}),
		"progress_monitor":   fixtures["adjacent-progress-bars"],
		"scrolling_table":    fixtures["scrolling-table"],
		"full_screen_redraw": fixtures["full-screen-redraw"],
	}
}
