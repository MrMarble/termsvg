package raster

import (
	"image/color"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

func TestNew(t *testing.T) {
	config := DefaultConfig()
	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	if r.config.FontSize != config.FontSize {
		t.Errorf("FontSize = %v, want %v", r.config.FontSize, config.FontSize)
	}
	if r.fontFace == nil {
		t.Error("fontFace is nil")
	}
}

func TestRasterize_EmptyRecording(t *testing.T) {
	config := DefaultConfig()
	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	rec := &ir.Recording{
		Frames: []ir.Frame{},
	}

	_, err = r.Rasterize(rec)
	if err == nil {
		t.Error("expected error for empty recording")
	}
}

func TestRasterize_SingleFrame(t *testing.T) {
	config := DefaultConfig()
	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

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

	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatalf("Rasterize() error = %v", err)
	}

	if len(frames) != 1 {
		t.Errorf("expected 1 frame, got %d", len(frames))
	}

	if frames[0].Image == nil {
		t.Error("frame.Image is nil")
	}

	if frames[0].Delay != 0 {
		t.Errorf("Delay = %v, want 0", frames[0].Delay)
	}

	if frames[0].Index != 0 {
		t.Errorf("Index = %v, want 0", frames[0].Index)
	}

	if frames[0].IsDuplicate {
		t.Error("first frame should not be marked as duplicate")
	}

	// Verify image dimensions
	expectedWidth := config.Padding*2 + rec.Width*config.ColWidth
	expectedHeight := config.Padding*config.HeaderSize + config.Padding + rec.Height*config.RowHeight
	bounds := frames[0].Image.Bounds()
	if bounds.Dx() != expectedWidth {
		t.Errorf("image width = %d, want %d", bounds.Dx(), expectedWidth)
	}
	if bounds.Dy() != expectedHeight {
		t.Errorf("image height = %d, want %d", bounds.Dy(), expectedHeight)
	}
}

func TestRasterize_MultipleFrames(t *testing.T) {
	config := DefaultConfig()
	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

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

	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatalf("Rasterize() error = %v", err)
	}

	if len(frames) != 2 {
		t.Errorf("expected 2 frames, got %d", len(frames))
	}

	// Check delays
	if frames[0].Delay != 0 {
		t.Errorf("frame 0 delay = %v, want 0", frames[0].Delay)
	}
	if frames[1].Delay != 1*time.Second {
		t.Errorf("frame 1 delay = %v, want 1s", frames[1].Delay)
	}

	// Check indices
	if frames[0].Index != 0 {
		t.Errorf("frame 0 index = %v, want 0", frames[0].Index)
	}
	if frames[1].Index != 1 {
		t.Errorf("frame 1 index = %v, want 1", frames[1].Index)
	}
}

func TestRasterize_WithWindow(t *testing.T) {
	config := DefaultConfig()
	config.ShowWindow = true

	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

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

	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatalf("Rasterize() error = %v", err)
	}

	// With window: height = content + header + padding
	// content = 10 * 25 = 250
	// header = 20 * 2 = 40
	// padding = 20
	// total = 310
	expectedHeight := 10*config.RowHeight + config.Padding*config.HeaderSize + config.Padding
	bounds := frames[0].Image.Bounds()
	if bounds.Dy() != expectedHeight {
		t.Errorf("expected height %d, got %d", expectedHeight, bounds.Dy())
	}
}

func TestRasterize_WithoutWindow(t *testing.T) {
	config := DefaultConfig()
	config.ShowWindow = false

	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

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

	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatalf("Rasterize() error = %v", err)
	}

	// Without window: height = content + 2*padding
	expectedHeight := 10*config.RowHeight + 2*config.Padding
	bounds := frames[0].Image.Bounds()
	if bounds.Dy() != expectedHeight {
		t.Errorf("expected height %d, got %d", expectedHeight, bounds.Dy())
	}
}

func TestRasterize_IRDeduplication(t *testing.T) {
	config := DefaultConfig()
	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	// Create frames where frames 1 and 3 are identical to their predecessors
	rec := &ir.Recording{
		Width:    40,
		Height:   10,
		Duration: 4 * time.Second,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 100 * time.Millisecond,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "A", StartCol: 0}}},
				},
			},
			{
				Time:  1 * time.Second,
				Delay: 100 * time.Millisecond,
				Index: 1,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "A", StartCol: 0}}}, // Same as frame 0
				},
			},
			{
				Time:  2 * time.Second,
				Delay: 100 * time.Millisecond,
				Index: 2,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "B", StartCol: 0}}}, // Different
				},
			},
			{
				Time:  3 * time.Second,
				Delay: 100 * time.Millisecond,
				Index: 3,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "B", StartCol: 0}}}, // Same as frame 2
				},
			},
		},
		Colors: colors,
	}

	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatalf("Rasterize() error = %v", err)
	}

	if len(frames) != 4 {
		t.Errorf("expected 4 frames, got %d", len(frames))
	}

	// Frame 0: not duplicate
	if frames[0].IsDuplicate {
		t.Error("frame 0 should not be a duplicate")
	}
	if frames[0].Image == nil {
		t.Error("frame 0 image should not be nil")
	}

	// Frame 1: duplicate of frame 0
	if !frames[1].IsDuplicate {
		t.Error("frame 1 should be marked as duplicate")
	}
	if frames[1].Image != nil {
		t.Error("frame 1 image should be nil (duplicate)")
	}

	// Frame 2: not duplicate
	if frames[2].IsDuplicate {
		t.Error("frame 2 should not be a duplicate")
	}
	if frames[2].Image == nil {
		t.Error("frame 2 image should not be nil")
	}

	// Frame 3: duplicate of frame 2
	if !frames[3].IsDuplicate {
		t.Error("frame 3 should be marked as duplicate")
	}
	if frames[3].Image != nil {
		t.Error("frame 3 image should be nil (duplicate)")
	}
}

func TestRasterize_WithStyling(t *testing.T) {
	config := DefaultConfig()
	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

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
								Text:     "Bold",
								StartCol: 0,
								Attrs:    ir.CellAttrs{Bold: true},
							},
							{
								Text:     "Italic",
								StartCol: 5,
								Attrs:    ir.CellAttrs{Italic: true},
							},
							{
								Text:     "Underline",
								StartCol: 12,
								Attrs:    ir.CellAttrs{Underline: true},
							},
							{
								Text:     "Dim",
								StartCol: 22,
								Attrs:    ir.CellAttrs{Dim: true},
							},
						},
					},
				},
				Cursor: ir.Cursor{Visible: false},
			},
		},
		Colors: colors,
	}

	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatalf("Rasterize() error = %v", err)
	}

	if len(frames) != 1 {
		t.Errorf("expected 1 frame, got %d", len(frames))
	}

	if frames[0].Image == nil {
		t.Error("frame.Image is nil")
	}
}

func TestRasterize_WithCursor(t *testing.T) {
	config := DefaultConfig()
	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

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
					{Y: 0, Runs: []ir.TextRun{{Text: "Hello", StartCol: 0}}},
				},
				Cursor: ir.Cursor{Col: 5, Row: 0, Visible: true},
			},
		},
		Colors: colors,
	}

	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatalf("Rasterize() error = %v", err)
	}

	if len(frames) != 1 {
		t.Errorf("expected 1 frame, got %d", len(frames))
	}

	if frames[0].Image == nil {
		t.Error("frame.Image is nil")
	}
}

func TestRasterize_MultipleRows(t *testing.T) {
	config := DefaultConfig()
	r, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

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
					{Y: 0, Runs: []ir.TextRun{{Text: "Line 1", StartCol: 0}}},
					{Y: 1, Runs: []ir.TextRun{{Text: "Line 2", StartCol: 0}}},
					{Y: 2, Runs: []ir.TextRun{{Text: "Line 3", StartCol: 0}}},
				},
				Cursor: ir.Cursor{Visible: false},
			},
		},
		Colors: colors,
	}

	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatalf("Rasterize() error = %v", err)
	}

	if len(frames) != 1 {
		t.Errorf("expected 1 frame, got %d", len(frames))
	}

	if frames[0].Image == nil {
		t.Error("frame.Image is nil")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.FontSize != 20 {
		t.Errorf("FontSize = %v, want 20", config.FontSize)
	}
	if !config.ShowWindow {
		t.Error("ShowWindow should be true by default")
	}
	if config.RowHeight != RowHeight {
		t.Errorf("RowHeight = %v, want %v", config.RowHeight, RowHeight)
	}
	if config.ColWidth != ColWidth {
		t.Errorf("ColWidth = %v, want %v", config.ColWidth, ColWidth)
	}
	if config.Padding != Padding {
		t.Errorf("Padding = %v, want %v", config.Padding, Padding)
	}
	if config.HeaderSize != HeaderSize {
		t.Errorf("HeaderSize = %v, want %v", config.HeaderSize, HeaderSize)
	}
}

func TestDimColor(t *testing.T) {
	tests := []struct {
		input    color.RGBA
		expected color.RGBA
	}{
		{
			input:    color.RGBA{R: 255, G: 255, B: 255, A: 255},
			expected: color.RGBA{R: 127, G: 127, B: 127, A: 255},
		},
		{
			input:    color.RGBA{R: 0, G: 0, B: 0, A: 255},
			expected: color.RGBA{R: 0, G: 0, B: 0, A: 255},
		},
		{
			input:    color.RGBA{R: 100, G: 150, B: 200, A: 255},
			expected: color.RGBA{R: 50, G: 75, B: 100, A: 255},
		},
	}

	for _, tt := range tests {
		result := dimColor(tt.input)
		if result != tt.expected {
			t.Errorf("dimColor(%v) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestFramesEqualIR(t *testing.T) {
	frame1 := &ir.Frame{
		Cursor: ir.Cursor{Col: 0, Row: 0, Visible: true},
		Rows: []ir.Row{
			{Y: 0, Runs: []ir.TextRun{{Text: "Hello", StartCol: 0, Attrs: ir.CellAttrs{}}}},
		},
	}

	frame2 := &ir.Frame{
		Cursor: ir.Cursor{Col: 0, Row: 0, Visible: true},
		Rows: []ir.Row{
			{Y: 0, Runs: []ir.TextRun{{Text: "Hello", StartCol: 0, Attrs: ir.CellAttrs{}}}},
		},
	}

	frame3 := &ir.Frame{
		Cursor: ir.Cursor{Col: 1, Row: 0, Visible: true}, // Different cursor
		Rows: []ir.Row{
			{Y: 0, Runs: []ir.TextRun{{Text: "Hello", StartCol: 0, Attrs: ir.CellAttrs{}}}},
		},
	}

	if !framesEqualIR(frame1, frame2) {
		t.Error("framesEqualIR should return true for identical frames")
	}

	if framesEqualIR(frame1, frame3) {
		t.Error("framesEqualIR should return false for different frames")
	}
}
