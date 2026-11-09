package webm

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"log"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/raster"
	"github.com/mrmarble/termsvg/internal/renderer"
)

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
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 1", StartCol: 0}}},
				},
			},
			{
				Time:  500 * time.Millisecond,
				Delay: 500 * time.Millisecond,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 2", StartCol: 0}}},
				},
			},
			{
				Time:  1 * time.Second,
				Delay: 500 * time.Millisecond,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 3", StartCol: 0}}},
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

func TestRenderer_Render_DebugLogsSummary(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetFlags(log.LstdFlags) })

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

	if want := "[WebM] 1 frames"; !strings.Contains(logs.String(), want) {
		t.Errorf("debug log = %q, want it to contain %q", logs.String(), want)
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
	rasterizer, err := raster.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &Renderer{config: *cfg, rasterizer: rasterizer}
}

func TestFilterFrames(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	ms := time.Millisecond
	tests := []struct {
		name       string
		in         []raster.RasterFrame
		wantDelays []time.Duration
	}{
		{
			name:       "long frames are kept",
			in:         []raster.RasterFrame{{Image: img, Delay: 100 * ms}, {Image: img, Delay: 50 * ms}},
			wantDelays: []time.Duration{100 * ms, 50 * ms},
		},
		{
			name: "rapid frames fold into the next one",
			in: []raster.RasterFrame{
				{Image: img, Delay: 10 * ms},
				{Image: img, Delay: 10 * ms},
				{Image: img, Delay: 100 * ms},
			},
			wantDelays: []time.Duration{120 * ms},
		},
		{
			name:       "a rapid last frame is still kept",
			in:         []raster.RasterFrame{{Image: img, Delay: 100 * ms}, {Image: img, Delay: 5 * ms}},
			wantDelays: []time.Duration{100 * ms, 5 * ms},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []time.Duration
			for _, f := range filterFrames(tt.in) {
				got = append(got, f.Delay)
			}
			if !slices.Equal(got, tt.wantDelays) {
				t.Errorf("delays = %v, want %v", got, tt.wantDelays)
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

func TestRenderer_Render_IsDeterministic(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Skipf("FFmpeg not installed: %v", err)
	}
	rec := &ir.Recording{
		Width: 20, Height: 3, Duration: time.Second,
		Colors: termcolor.NewCatalog(color.RGBA{R: 192, G: 192, B: 192, A: 255}, color.RGBA{A: 255}),
		Frames: []ir.Frame{
			{
				Delay: 200 * time.Millisecond,
				Rows:  []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "first", StartCol: 0}}}},
			},
			{
				Time: 200 * time.Millisecond, Delay: 200 * time.Millisecond,
				Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "second", StartCol: 0}}}},
			},
		},
	}

	var first []byte
	for i := range 3 {
		var buf bytes.Buffer
		if err := r.Render(context.Background(), rec, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		if i == 0 {
			first = buf.Bytes()
			continue
		}
		if !bytes.Equal(first, buf.Bytes()) {
			t.Fatalf("render %d differs from the first (%d vs %d bytes); output must be reproducible",
				i+1, buf.Len(), len(first))
		}
	}
}
