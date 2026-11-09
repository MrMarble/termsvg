package raster

import (
	"image/color"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
)

func TestNew(t *testing.T) {
	config := testConfig()
	r, err := New(&config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if r.config.FontSize != config.FontSize {
		t.Errorf("FontSize = %v, want %v", r.config.FontSize, config.FontSize)
	}
}

func TestRasterize_EmptyRecording(t *testing.T) {
	config := testConfig()
	r, err := New(&config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := &ir.Recording{
		Frames: []ir.Frame{},
	}

	_, err = r.Rasterize(rec)
	if err == nil {
		t.Error("expected error for empty recording")
	}
}

//nolint:funlen // test setup requires multiple test cases
func TestRasterize_SingleFrame(t *testing.T) {
	config := testConfig()
	r, err := New(&config)
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

	// The only frame is also the last, so it is held.
	if frames[0].Delay != lastFrameHold {
		t.Errorf("Delay = %v, want the last-frame hold %v", frames[0].Delay, lastFrameHold)
	}

	// Verify image dimensions
	expectedWidth := Padding*2 + rec.Width*ColWidth
	expectedHeight := Padding*HeaderSize + Padding + rec.Height*RowHeight
	bounds := frames[0].Image.Bounds()
	if bounds.Dx() != expectedWidth {
		t.Errorf("image width = %d, want %d", bounds.Dx(), expectedWidth)
	}
	if bounds.Dy() != expectedHeight {
		t.Errorf("image height = %d, want %d", bounds.Dy(), expectedHeight)
	}
}

func TestRasterize_MultipleFrames(t *testing.T) {
	config := testConfig()
	r, err := New(&config)
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
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 1", StartCol: 0}}},
				},
			},
			{
				Time:  1 * time.Second,
				Delay: 1 * time.Second,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "Frame 2", StartCol: 0}}},
				},
			},
		},
		Colors: colors,
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
	if frames[1].Delay != lastFrameHold { // its own 1s is shorter than the hold given to the last frame
		t.Errorf("frame 1 delay = %v, want the last-frame hold %v", frames[1].Delay, lastFrameHold)
	}

}

func TestRasterize_WithWindow(t *testing.T) {
	config := testConfig()
	config.ShowWindow = true

	r, err := New(&config)
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
	expectedHeight := 10*RowHeight + Padding*HeaderSize + Padding
	bounds := frames[0].Image.Bounds()
	if bounds.Dy() != expectedHeight {
		t.Errorf("expected height %d, got %d", expectedHeight, bounds.Dy())
	}
}

func TestRasterize_WithoutWindow(t *testing.T) {
	config := testConfig()
	config.ShowWindow = false

	r, err := New(&config)
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
	expectedHeight := 10*RowHeight + 2*Padding
	bounds := frames[0].Image.Bounds()
	if bounds.Dy() != expectedHeight {
		t.Errorf("expected height %d, got %d", expectedHeight, bounds.Dy())
	}
}

//nolint:funlen // test requires comprehensive frame rendering scenario
func TestRasterize_MultipleUniqueFrames(t *testing.T) {
	config := testConfig()
	r, err := New(&config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	// Create 4 unique frames - rasterizer no longer handles deduplication
	// (that's done at the IR processing stage)
	rec := &ir.Recording{
		Width:    40,
		Height:   10,
		Duration: 4 * time.Second,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 100 * time.Millisecond,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "A", StartCol: 0}}},
				},
			},
			{
				Time:  1 * time.Second,
				Delay: 100 * time.Millisecond,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "A", StartCol: 0}}},
				},
			},
			{
				Time:  2 * time.Second,
				Delay: 100 * time.Millisecond,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "B", StartCol: 0}}},
				},
			},
			{
				Time:  3 * time.Second,
				Delay: 100 * time.Millisecond,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{{Text: "B", StartCol: 0}}},
				},
			},
		},
		Colors: colors,
	}

	frames, err := r.Rasterize(rec)
	if err != nil {
		t.Fatalf("Rasterize() error = %v", err)
	}

	// All 4 frames should be rendered (no deduplication at raster level)
	if len(frames) != 4 {
		t.Errorf("expected 4 frames, got %d", len(frames))
	}

	// All frames should have images (no duplicates marked at raster level)
	for i, frame := range frames {
		if frame.Image == nil {
			t.Errorf("frame %d image should not be nil", i)
		}
	}
}

//nolint:funlen // test requires multiple styling scenarios
func TestRasterize_WithStyling(t *testing.T) {
	config := testConfig()
	r, err := New(&config)
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
	config := testConfig()
	r, err := New(&config)
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
	config := testConfig()
	r, err := New(&config)
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

func TestDisplayDelay(t *testing.T) {
	frames := []ir.Frame{{Delay: time.Second}, {Delay: 0}, {Delay: 100 * time.Millisecond}}
	tests := []struct {
		idx  int
		want time.Duration
	}{
		{idx: 0, want: time.Second},                 // a frame lasts until the next one
		{idx: 1, want: 0},                           // however briefly
		{idx: 2, want: lastFrameHold},               // the last frame is held so it can be read
		{idx: len(frames) - 1, want: lastFrameHold}, // (the same frame)
	}
	for _, tt := range tests {
		if got := displayDelay(frames, tt.idx); got != tt.want {
			t.Errorf("displayDelay(%d) = %v, want %v", tt.idx, got, tt.want)
		}
	}

	// A last frame that already lasts longer than the hold keeps its time.
	long := []ir.Frame{{Delay: time.Second}, {Delay: 5 * time.Second}}
	if got := displayDelay(long, 1); got != 5*time.Second {
		t.Errorf("displayDelay = %v, want 5s", got)
	}
}

func rasterTestRecording() *ir.Recording {
	return &ir.Recording{
		Width: 10, Height: 2, Duration: 6 * time.Second,
		Colors: termcolor.NewCatalog(color.RGBA{R: 192, G: 192, B: 192, A: 255}, color.RGBA{A: 255}),
		Frames: []ir.Frame{
			{
				Time: 500 * time.Millisecond, Delay: 3 * time.Second,
				Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "A"}}}},
			},
			{
				Time: 3500 * time.Millisecond, Delay: 2 * time.Second,
				Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "AB"}}}},
			},
			{
				Time: 5500 * time.Millisecond, Delay: 0,
				Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "ABC"}}}},
			},
		},
	}
}

func TestRasterize_FramesLastAsLongAsTheyAreOnScreen(t *testing.T) {
	cfg := testConfig()
	r, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{3 * time.Second, 2 * time.Second, lastFrameHold}

	rgba, err := r.Rasterize(rasterTestRecording())
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range rgba {
		if f.Delay != want[i] {
			t.Errorf("RGBA frame %d Delay = %v, want %v", i, f.Delay, want[i])
		}
	}

	paletted, err := r.RasterizeWithPalette(rasterTestRecording(), testPalette(&cfg))
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range paletted {
		if f.Delay != want[i] {
			t.Errorf("paletted frame %d Delay = %v, want %v", i, f.Delay, want[i])
		}
	}
}
