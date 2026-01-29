package svg

import (
	"bytes"
	"context"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestNew(t *testing.T) {
	config := renderer.DefaultConfig()
	r := New(config)

	if r == nil {
		t.Fatal("New() returned nil")
	}
	if r.Format() != "svg" {
		t.Errorf("Format() = %q, want %q", r.Format(), "svg")
	}
	if r.FileExtension() != ".svg" {
		t.Errorf("FileExtension() = %q, want %q", r.FileExtension(), ".svg")
	}
}

func TestRender_EmptyRecording(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := &ir.Recording{
		Width:  80,
		Height: 24,
		Frames: []ir.Frame{},
		Colors: termcolor.NewColorCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
	}

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)

	if err == nil {
		t.Error("expected error for empty recording, got nil")
	}
}

func TestRender_BasicStructure(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := createTestRecording()

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Check basic SVG structure
	checks := []string{
		`<svg xmlns="http://www.w3.org/2000/svg"`,
		`</svg>`,
		`<style>`,
		`</style>`,
		`@keyframes k`,
		`<text`,
	}

	for _, check := range checks {
		if !strings.Contains(svg, check) {
			t.Errorf("SVG missing expected element: %q", check)
		}
	}
}

func TestRender_WindowChrome(t *testing.T) {
	config := renderer.DefaultConfig()
	config.ShowWindow = true
	r := New(config)
	rec := createTestRecording()

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Should have rounded rect for window
	if !strings.Contains(svg, `rx="5"`) {
		t.Error("SVG missing window rounded corners")
	}

	// Should have window buttons (circles)
	if !strings.Contains(svg, `<circle`) {
		t.Error("SVG missing window buttons")
	}
}

func TestRender_NoWindowChrome(t *testing.T) {
	config := renderer.DefaultConfig()
	config.ShowWindow = false
	r := New(config)
	rec := createTestRecording()

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Should not have window buttons
	if strings.Contains(svg, `<circle`) {
		t.Error("SVG should not have window buttons when ShowWindow=false")
	}
}

func TestRender_Keyframes(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := createTestRecording()

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Check keyframes exist
	if !strings.Contains(svg, "@keyframes k{") {
		t.Error("SVG missing keyframes animation")
	}

	// Check animation is applied
	if !strings.Contains(svg, "animation:k") {
		t.Error("SVG missing animation style")
	}
}

func TestRender_ColorClasses(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := createTestRecording()

	// Register a specific color
	redID := rec.Colors.Register(termcolor.FromRGB(255, 0, 0), termcolor.Standard())

	// Add a frame with that color
	rec.Frames = append(rec.Frames, ir.Frame{
		Time:  2 * time.Second,
		Delay: time.Second,
		Index: 2,
		Rows: []ir.Row{
			{Y: 0, Runs: []ir.TextRun{
				{Text: "Red", StartCol: 0, Attrs: ir.CellAttrs{FG: redID}},
			}},
		},
	})
	rec.Duration = 2 * time.Second

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Should have color class definition
	if !strings.Contains(svg, "#FF0000") {
		t.Error("SVG missing red color definition")
	}

	// Should have class applied to text
	if !strings.Contains(svg, `class="`) {
		t.Error("SVG missing class attribute on text")
	}
}

func TestRender_TextAttributes(t *testing.T) {
	config := renderer.DefaultConfig()
	r := New(config)

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: time.Second,
		Colors: termcolor.NewColorCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
		Frames: []ir.Frame{
			{
				Time:  0,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{
						{Text: "Bold", StartCol: 0, Attrs: ir.CellAttrs{Bold: true}},
						{Text: "Italic", StartCol: 5, Attrs: ir.CellAttrs{Italic: true}},
						{Text: "Underline", StartCol: 12, Attrs: ir.CellAttrs{Underline: true}},
						{Text: "Dim", StartCol: 22, Attrs: ir.CellAttrs{Dim: true}},
					}},
				},
			},
		},
		Stats: ir.Stats{
			HasBold:      true,
			HasItalic:    true,
			HasUnderline: true,
			HasDim:       true,
		},
	}

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Check attribute classes exist
	if !strings.Contains(svg, ".bold{font-weight:bold}") {
		t.Error("SVG missing bold class")
	}
	if !strings.Contains(svg, ".italic{font-style:italic}") {
		t.Error("SVG missing italic class")
	}
	if !strings.Contains(svg, ".underline{text-decoration:underline}") {
		t.Error("SVG missing underline class")
	}
	if !strings.Contains(svg, ".dim{opacity:0.5}") {
		t.Error("SVG missing dim class")
	}

	// Check classes are applied to text
	if !strings.Contains(svg, `class="bold"`) {
		t.Error("SVG missing bold class on text")
	}
}

func TestRender_BackgroundFilters(t *testing.T) {
	config := renderer.DefaultConfig()
	r := New(config)

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: time.Second,
		Colors: termcolor.NewColorCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
		Frames: []ir.Frame{},
	}

	// Register a background color
	bgID := rec.Colors.Register(termcolor.FromRGB(0, 0, 255), termcolor.Standard())

	rec.Frames = []ir.Frame{
		{
			Time:  0,
			Index: 0,
			Rows: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{
					{Text: "WithBG", StartCol: 0, Attrs: ir.CellAttrs{BG: bgID}},
				}},
			},
		},
	}

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Check filter is defined
	if !strings.Contains(svg, `<filter id="bg_`) {
		t.Error("SVG missing background filter definition")
	}
	if !strings.Contains(svg, `feFlood`) {
		t.Error("SVG missing feFlood in filter")
	}

	// Check filter is applied
	if !strings.Contains(svg, `filter="url(#bg_`) {
		t.Error("SVG missing filter reference on text")
	}
}

func TestRender_HTMLEscaping(t *testing.T) {
	r := New(renderer.DefaultConfig())

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: time.Second,
		Colors: termcolor.NewColorCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
		Frames: []ir.Frame{
			{
				Time:  0,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{
						{Text: "<script>alert('xss')</script>", StartCol: 0},
					}},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Should escape HTML
	if strings.Contains(svg, "<script>") {
		t.Error("SVG contains unescaped script tag")
	}
	if !strings.Contains(svg, "&lt;script&gt;") {
		t.Error("SVG missing escaped script tag")
	}
}

func TestRender_LoopCount(t *testing.T) {
	tests := []struct {
		name      string
		loopCount int
		want      string
	}{
		{"infinite", 0, "infinite"},
		{"no loop", -1, "1"},
		{"specific count", 3, "3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := renderer.DefaultConfig()
			config.LoopCount = tt.loopCount
			r := New(config)
			rec := createTestRecording()

			var buf bytes.Buffer
			err := r.Render(context.Background(), rec, &buf)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			svg := buf.String()
			expected := tt.want + " steps(1,end)"
			if !strings.Contains(svg, expected) {
				t.Errorf("SVG missing loop count %q", expected)
			}
		})
	}
}

func TestRender_ContextCancellation(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := createTestRecording()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	err := r.Render(ctx, rec, &buf)

	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestCanvas_Dimensions(t *testing.T) {
	config := renderer.DefaultConfig()
	rec := &ir.Recording{
		Width:  80,
		Height: 24,
		Colors: termcolor.NewColorCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
	}

	c := &canvas{
		rec:    rec,
		config: config,
	}

	// Content dimensions
	if c.contentWidth() != 80*ColWidth {
		t.Errorf("contentWidth() = %d, want %d", c.contentWidth(), 80*ColWidth)
	}
	if c.contentHeight() != 24*RowHeight {
		t.Errorf("contentHeight() = %d, want %d", c.contentHeight(), 24*RowHeight)
	}

	// Padded dimensions with window
	config.ShowWindow = true
	c.config = config
	expectedPaddedHeight := c.contentHeight() + Padding*HeaderSize + Padding // header + bottom padding
	if c.paddedHeight() != expectedPaddedHeight {
		t.Errorf("paddedHeight() with window = %d, want %d", c.paddedHeight(), expectedPaddedHeight)
	}

	// Padded dimensions without window
	config.ShowWindow = false
	c.config = config
	expectedPaddedHeight = c.contentHeight() + 2*Padding
	if c.paddedHeight() != expectedPaddedHeight {
		t.Errorf("paddedHeight() without window = %d, want %d", c.paddedHeight(), expectedPaddedHeight)
	}
}

// createTestRecording creates a simple recording for testing
func createTestRecording() *ir.Recording {
	colors := termcolor.NewColorCatalog(
		color.RGBA{R: 255, G: 255, B: 255, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	return &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: time.Second,
		Title:    "Test Recording",
		Colors:   colors,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 0,
				Index: 0,
				Rows: []ir.Row{
					{
						Y: 0,
						Runs: []ir.TextRun{
							{Text: "Hello", StartCol: 0},
							{Text: "World", StartCol: 6},
						},
					},
				},
			},
			{
				Time:  500 * time.Millisecond,
				Delay: 500 * time.Millisecond,
				Index: 1,
				Rows: []ir.Row{
					{
						Y: 0,
						Runs: []ir.TextRun{
							{Text: "Goodbye", StartCol: 0},
						},
					},
				},
			},
		},
		Stats: ir.Stats{
			TotalFrames: 2,
		},
	}
}

// Integration tests using example files

func TestIntegration_256Colors(t *testing.T) {
	// Find the examples directory (relative to this test file)
	examplesDir := filepath.Join("..", "..", "..", "examples")
	castPath := filepath.Join(examplesDir, "256colors.cast")

	// Skip if example file doesn't exist
	if _, err := os.Stat(castPath); os.IsNotExist(err) {
		t.Skipf("Example file not found: %s", castPath)
	}

	// Load the cast file
	f, err := os.Open(castPath)
	if err != nil {
		t.Fatalf("Failed to open cast file: %v", err)
	}
	defer f.Close()

	cast, err := asciicast.Parse(f)
	if err != nil {
		t.Fatalf("Failed to parse cast file: %v", err)
	}

	// Process through IR
	proc := ir.NewProcessor(ir.DefaultProcessorConfig())
	rec, err := proc.Process(cast)
	if err != nil {
		t.Fatalf("Failed to process cast: %v", err)
	}

	// Verify IR was generated correctly
	if rec.Width != 120 {
		t.Errorf("Recording width = %d, want 120", rec.Width)
	}
	if rec.Height != 42 {
		t.Errorf("Recording height = %d, want 42", rec.Height)
	}
	if len(rec.Frames) == 0 {
		t.Error("Recording has no frames")
	}

	// Render to SVG
	r := New(renderer.DefaultConfig())
	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Failed to render SVG: %v", err)
	}

	svg := buf.String()

	// Verify SVG structure
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("Output doesn't start with <svg")
	}
	if !strings.HasSuffix(svg, "</svg>") {
		t.Error("Output doesn't end with </svg>")
	}

	// Verify it contains expected elements for 256 color test
	if !strings.Contains(svg, "@keyframes") {
		t.Error("SVG missing keyframes animation")
	}
	if !strings.Contains(svg, "<style>") {
		t.Error("SVG missing style element")
	}
	if !strings.Contains(svg, "<text") {
		t.Error("SVG missing text elements")
	}

	// Verify multiple color classes were generated (256 color demo should have many)
	if rec.Stats.UniqueColors < 10 {
		t.Errorf("Expected many unique colors for 256color demo, got %d", rec.Stats.UniqueColors)
	}

	// Verify background filters exist (256 color demo uses background colors)
	if !strings.Contains(svg, `<filter id="bg_`) {
		t.Error("SVG missing background filters for 256 color demo")
	}

	t.Logf("Generated SVG: %d bytes, %d frames, %d unique colors",
		len(svg), rec.Stats.TotalFrames, rec.Stats.UniqueColors)

	f, err = os.OpenFile(filepath.Join(examplesDir, "256colors.svg"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("Failed to create output SVG file: %v", err)
	}
	defer f.Close()
	_, err = f.WriteString(svg)
	if err != nil {
		t.Fatalf("Failed to write SVG to file: %v", err)
	}
}
