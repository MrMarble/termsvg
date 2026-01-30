package gif

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"image/gif"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/raster"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestRenderer_Format(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got := r.Format(); got != "gif" {
		t.Errorf("Format() = %v, want %v", got, "gif")
	}
}

func TestRenderer_FileExtension(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got := r.FileExtension(); got != ".gif" {
		t.Errorf("FileExtension() = %v, want %v", got, ".gif")
	}
}

func TestRenderer_Render_EmptyRecording(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
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
		t.Fatalf("New() error = %v", err)
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

	// Verify it's a valid GIF
	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("failed to decode GIF: %v", err)
	}

	if len(g.Image) != 1 {
		t.Errorf("expected 1 frame, got %d", len(g.Image))
	}
}

func TestRenderer_Render_MultipleFrames(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: 2 * time.Second,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 0,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 1", StartCol: 0}}},
				},
			},
			{
				Time:  1 * time.Second,
				Delay: 1 * time.Second,
				Index: 1,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 2", StartCol: 0}}},
				},
			},
		},
		Colors: colors,
		Stats:  ir.Stats{TotalFrames: 2},
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("failed to decode GIF: %v", err)
	}

	if len(g.Image) != 2 {
		t.Errorf("expected 2 frames, got %d", len(g.Image))
	}

	// Check frame delay (1 second = 100 units of 10ms)
	if g.Delay[1] != 100 {
		t.Errorf("expected delay of 100 (1s), got %d", g.Delay[1])
	}
}

func TestRenderer_Render_WithWindow(t *testing.T) {
	config := renderer.DefaultConfig()
	config.ShowWindow = true

	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
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
				Rows:  []ir.Row{},
			},
		},
		Colors: colors,
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("failed to decode GIF: %v", err)
	}

	// With window: height = content + header + padding
	// content = 10 * 25 = 250
	// header = 20 * 2 = 40
	// padding = 20
	// total = 310
	expectedHeight := 10*raster.RowHeight + raster.Padding*raster.HeaderSize + raster.Padding
	if g.Image[0].Bounds().Dy() != expectedHeight {
		t.Errorf("expected height %d, got %d", expectedHeight, g.Image[0].Bounds().Dy())
	}
}

func TestRenderer_Render_WithoutWindow(t *testing.T) {
	config := renderer.DefaultConfig()
	config.ShowWindow = false

	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
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
				Rows:  []ir.Row{},
			},
		},
		Colors: colors,
	}

	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("failed to decode GIF: %v", err)
	}

	// Without window: height = content + 2*padding
	expectedHeight := 10*raster.RowHeight + 2*raster.Padding
	if g.Image[0].Bounds().Dy() != expectedHeight {
		t.Errorf("expected height %d, got %d", expectedHeight, g.Image[0].Bounds().Dy())
	}
}

func TestRenderer_Render_ContextCancellation(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	// Create a recording with many frames
	frames := make([]ir.Frame, 100)
	for i := range frames {
		frames[i] = ir.Frame{
			Time:  time.Duration(i) * 100 * time.Millisecond,
			Delay: 100 * time.Millisecond,
			Index: i,
			Rows:  []ir.Row{},
		}
	}

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: 10 * time.Second,
		Frames:   frames,
		Colors:   colors,
	}

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	err = r.Render(ctx, rec, &buf)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}
