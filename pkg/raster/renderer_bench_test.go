package raster

import (
	"image/color"
	"testing"
	"time"

	irColor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/theme"
)

// createBenchmarkRecording creates a recording with the specified number of frames
// for benchmarking purposes.
func createBenchmarkRecording(numFrames, width, height int) *ir.Recording {
	frames := make([]ir.Frame, numFrames)
	for i := range frames {
		frames[i] = ir.Frame{
			Index: i,
			Delay: 100 * time.Millisecond,
			Rows: []ir.Row{
				{
					Y: 0,
					Runs: []ir.TextRun{
						{
							Text:     "Benchmark test content for frame rendering performance",
							StartCol: 0,
							Attrs: ir.CellAttrs{
								FG: 7, // White
								BG: 0, // Black
							},
						},
					},
				},
			},
			Cursor: ir.Cursor{
				Visible: true,
				Col:     10,
				Row:     0,
			},
		}
	}

	return &ir.Recording{
		Width:  width,
		Height: height,
		Frames: frames,
		Colors: irColor.NewCatalog(color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 255}),
	}
}

// BenchmarkRasterize_10Frames benchmarks rendering 10 frames.
func BenchmarkRasterize_10Frames(b *testing.B) {
	rec := createBenchmarkRecording(10, 80, 24)
	config := Config{
		Theme:      theme.Default(),
		ShowWindow: false,
		ShowCursor: true,
		FontSize:   14,
	}
	r, err := New(config)
	if err != nil {
		b.Fatalf("failed to create rasterizer: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Rasterize(rec)
	}
}

// BenchmarkRasterize_50Frames benchmarks rendering 50 frames.
func BenchmarkRasterize_50Frames(b *testing.B) {
	rec := createBenchmarkRecording(50, 80, 24)
	config := Config{
		Theme:      theme.Default(),
		ShowWindow: false,
		ShowCursor: true,
		FontSize:   14,
	}
	r, err := New(config)
	if err != nil {
		b.Fatalf("failed to create rasterizer: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Rasterize(rec)
	}
}

// BenchmarkRasterize_100Frames benchmarks rendering 100 frames.
func BenchmarkRasterize_100Frames(b *testing.B) {
	rec := createBenchmarkRecording(100, 80, 24)
	config := Config{
		Theme:      theme.Default(),
		ShowWindow: false,
		ShowCursor: true,
		FontSize:   14,
	}
	r, err := New(config)
	if err != nil {
		b.Fatalf("failed to create rasterizer: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Rasterize(rec)
	}
}

// BenchmarkRasterize_200Frames benchmarks rendering 200 frames.
func BenchmarkRasterize_200Frames(b *testing.B) {
	rec := createBenchmarkRecording(200, 80, 24)
	config := Config{
		Theme:      theme.Default(),
		ShowWindow: false,
		ShowCursor: true,
		FontSize:   14,
	}
	r, err := New(config)
	if err != nil {
		b.Fatalf("failed to create rasterizer: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Rasterize(rec)
	}
}

// BenchmarkRasterize_WithWindow benchmarks rendering with window chrome.
func BenchmarkRasterize_WithWindow(b *testing.B) {
	rec := createBenchmarkRecording(50, 80, 24)
	config := Config{
		Theme:      theme.Default(),
		ShowWindow: true,
		ShowCursor: true,
		FontSize:   14,
	}
	r, err := New(config)
	if err != nil {
		b.Fatalf("failed to create rasterizer: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Rasterize(rec)
	}
}
