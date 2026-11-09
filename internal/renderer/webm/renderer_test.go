package webm

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/raster"
	"github.com/mrmarble/termsvg/internal/renderer"
)

func TestRenderer_Format(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	if got := r.Format(); got != "webm" {
		t.Errorf("Format() = %v, want %v", got, "webm")
	}
}

func TestRenderer_FileExtension(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	if got := r.FileExtension(); got != ".webm" {
		t.Errorf("FileExtension() = %v, want %v", got, ".webm")
	}
}

func TestRenderer_Render_EmptyRecording(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	rec := &ir.Recording{
		Frames: []ir.Frame{},
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err == nil {
		t.Error("expected error for empty recording")
	}
}

func TestRenderer_Render_SingleFrame(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: 1 * time.Second,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 0,
				Index: 0,
				Rows: []ir.Row{
					{
						Y: 0,
						Runs: []ir.TextRun{
							{
								Text:     "Hello, World!",
								StartCol: 0,
								Attrs:    ir.CellAttrs{},
							},
						},
					},
				},
				Cursor: ir.Cursor{Col: 13, Row: 0, Visible: true},
			},
		},
		Colors: colors,
		Stats:  ir.Stats{TotalFrames: 1},
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// WebM files start with specific bytes (EBML header)
	// Check that we got some output
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}

	// WebM files start with 0x1A 0x45 0xDF 0xA3 (EBML ID)
	data := buf.Bytes()
	if len(data) < 4 {
		t.Error("output too short")
		return
	}

	// Check for EBML header signature
	if data[0] != 0x1A || data[1] != 0x45 || data[2] != 0xDF || data[3] != 0xA3 {
		t.Errorf("output does not appear to be a valid WebM file (missing EBML header)")
	}
}

func TestRenderer_Render_MultipleFrames(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	rec := &ir.Recording{
		Width:    40,
		Height:   10,
		Duration: 2 * time.Second,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 500 * time.Millisecond,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 1", StartCol: 0}}},
				},
			},
			{
				Time:  500 * time.Millisecond,
				Delay: 500 * time.Millisecond,
				Index: 1,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 2", StartCol: 0}}},
				},
			},
			{
				Time:  1 * time.Second,
				Delay: 500 * time.Millisecond,
				Index: 2,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 3", StartCol: 0}}},
				},
			},
		},
		Colors: colors,
		Stats:  ir.Stats{TotalFrames: 3},
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestRenderer_Render_WithDebug(t *testing.T) {
	config := renderer.DefaultConfig()
	config.Debug = true

	r, err := New(config)
	if err != nil {
		// FFmpeg might not be installed, skip test
		t.Skipf("FFmpeg not installed: %v", err)
	}

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	rec := &ir.Recording{
		Width:    40,
		Height:   10,
		Duration: 1 * time.Second,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 0,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Test", StartCol: 0}}},
				},
			},
		},
		Colors: colors,
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestNew_WithoutFFmpeg(t *testing.T) {
	// An empty PATH guarantees ffmpeg cannot be found, whatever the host has.
	t.Setenv("PATH", t.TempDir())

	r, err := New(renderer.DefaultConfig())
	if err == nil || r != nil {
		t.Fatalf("New() = %v, %v; want an error", r, err)
	}
	if want := "ffmpeg is not installed. Install it from: https://ffmpeg.org"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// newUnprobedRenderer builds a Renderer without looking for ffmpeg, so the
// code paths that never reach the encoder can be tested on any machine.
func newUnprobedRenderer(t *testing.T) *Renderer {
	t.Helper()
	cfg := renderer.DefaultConfig()
	rasterizer, err := renderer.NewRasterizer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &Renderer{config: *cfg, rasterizer: rasterizer}
}

func TestRenderer_FilterFrames(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	ms := time.Millisecond
	tests := []struct {
		name       string
		in         []raster.RasterFrame
		wantDelays []time.Duration
		wantIndex  []int
	}{
		{
			name:       "long frames are kept",
			in:         []raster.RasterFrame{{Image: img, Delay: 100 * ms, Index: 0}, {Image: img, Delay: 50 * ms, Index: 1}},
			wantDelays: []time.Duration{100 * ms, 50 * ms}, wantIndex: []int{0, 1},
		},
		{
			name: "rapid frames fold into the next one",
			in: []raster.RasterFrame{
				{Image: img, Delay: 10 * ms, Index: 0},
				{Image: img, Delay: 10 * ms, Index: 1},
				{Image: img, Delay: 100 * ms, Index: 2},
			},
			wantDelays: []time.Duration{120 * ms}, wantIndex: []int{2},
		},
		{
			name:       "a rapid last frame is still kept",
			in:         []raster.RasterFrame{{Image: img, Delay: 100 * ms, Index: 0}, {Image: img, Delay: 5 * ms, Index: 1}},
			wantDelays: []time.Duration{100 * ms, 5 * ms}, wantIndex: []int{0, 1},
		},
		{
			name:       "nil frames donate their delay",
			in:         []raster.RasterFrame{{Delay: 40 * ms, Index: 0}, {Image: img, Delay: 10 * ms, Index: 1}},
			wantDelays: []time.Duration{50 * ms}, wantIndex: []int{1},
		},
		{name: "empty input", in: nil},
	}
	r := &Renderer{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.filterFrames(tt.in)
			if len(got) != len(tt.wantDelays) {
				t.Fatalf("frames = %d, want %d", len(got), len(tt.wantDelays))
			}
			for i, f := range got {
				if f.Delay != tt.wantDelays[i] || f.Index != tt.wantIndex[i] {
					t.Errorf("frame %d: delay=%v index=%d, want %v/%d", i, f.Delay, f.Index, tt.wantDelays[i], tt.wantIndex[i])
				}
			}
		})
	}
}

func TestRenderer_EncodeToWebM_RejectsUnusableFrames(t *testing.T) {
	r := &Renderer{}
	tests := []struct {
		name   string
		frames []raster.RasterFrame
	}{
		{name: "no frames", frames: nil},
		{name: "only nil images", frames: []raster.RasterFrame{{Delay: time.Second}, {Delay: time.Second}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := r.encodeToWebM(tt.frames, &bytes.Buffer{}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestRenderer_Render_CancelledBeforeEncoding(t *testing.T) {
	r := newUnprobedRenderer(t)
	rec := &ir.Recording{
		Width: 10, Height: 2, Colors: termcolor.NewCatalog(
			color.RGBA{R: 192, G: 192, B: 192, A: 255}, color.RGBA{A: 255}),
		Frames: []ir.Frame{{Delay: time.Second}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := r.Render(ctx, rec, &bytes.Buffer{}); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}
