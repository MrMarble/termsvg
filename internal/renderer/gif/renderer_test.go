package gif

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"log"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/raster"
	"github.com/mrmarble/termsvg/internal/renderer"
)

type errWriter struct{ err error }

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

	// The last frame is held for 2 seconds (200 units of 10ms) so it can be read,
	// although it was only on screen for 1 second in the recording.
	if g.Delay[1] != 200 {
		t.Errorf("expected the last frame to be held for 200 (2s), got %d", g.Delay[1])
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

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

func gifTestRecording(frames ...ir.Frame) *ir.Recording {
	colors := termcolor.NewCatalog(
		color.RGBA{R: 192, G: 192, B: 192, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)
	return &ir.Recording{Width: 20, Height: 4, Frames: frames, Colors: colors}
}

func textFrame(text string, delay time.Duration) ir.Frame {
	return ir.Frame{Delay: delay, Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: text}}}}}
}

func TestRenderer_Render_MergesIdenticalFrames(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	rec := gifTestRecording(
		textFrame("same", 100*time.Millisecond),
		textFrame("same", 200*time.Millisecond),
		textFrame("different", 300*time.Millisecond),
	)

	var buf bytes.Buffer
	if err := r.Render(context.Background(), rec, &buf); err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 2 {
		t.Fatalf("images = %d, want 2 (identical frames merged)", len(g.Image))
	}
	// 100ms + 200ms merged into one frame, in 10ms units; the last frame is
	// held for 2 seconds instead of its own 300ms.
	if g.Delay[0] != 30 || g.Delay[1] != 200 {
		t.Errorf("delays = %v, want [30 200]", g.Delay)
	}
}

func TestRenderer_Render_ReportsProgress(t *testing.T) {
	ch := make(chan progress.Update, 64)
	cfg := renderer.DefaultConfig()
	cfg.ProgressCh = ch
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rec := gifTestRecording(textFrame("a", 50*time.Millisecond), textFrame("b", 50*time.Millisecond))

	if err := r.Render(context.Background(), rec, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	close(ch)

	phases := map[string]int{}
	for u := range ch {
		phases[u.Phase]++
	}
	if phases["Rasterizing"] == 0 || phases["Encoding"] == 0 {
		t.Errorf("progress phases = %v, want both Rasterizing and Encoding", phases)
	}
}

func TestRenderer_Render_DebugLogsPhases(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetFlags(log.LstdFlags) })

	cfg := renderer.DefaultConfig()
	cfg.Debug = true
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rec := gifTestRecording(textFrame("a", 50*time.Millisecond), textFrame("b", 50*time.Millisecond))
	if err := r.Render(context.Background(), rec, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"Phase 1", "Phase 2", "Phase 3", "framesEqual", "computeDelta", "Total time"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("debug log missing %q:\n%s", want, logs.String())
		}
	}
}

func TestRenderer_Render_PropagatesWriteError(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	rec := gifTestRecording(textFrame("a", 50*time.Millisecond))
	want := errors.New("disk full")
	if err := r.Render(context.Background(), rec, errWriter{want}); !errors.Is(err, want) {
		t.Errorf("error = %v, want %v", err, want)
	}
}

func TestRenderer_calculateDelay(t *testing.T) {
	r := &Renderer{}
	tests := []struct {
		name         string
		delay        time.Duration
		idx, total   int
		wantGIFUnits int
	}{
		{name: "regular delay in 10ms units", delay: 250 * time.Millisecond, idx: 0, total: 3, wantGIFUnits: 25},
		{name: "short delay is raised to the minimum", delay: 10 * time.Millisecond, idx: 0, total: 3, wantGIFUnits: 2},
		{name: "zero delay is raised to the minimum", delay: 0, idx: 1, total: 3, wantGIFUnits: 2},
		{name: "last frame is not raised", delay: 10 * time.Millisecond, idx: 2, total: 3, wantGIFUnits: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.calculateDelay(tt.delay, tt.idx, tt.total); got != tt.wantGIFUnits {
				t.Errorf("calculateDelay() = %d, want %d", got, tt.wantGIFUnits)
			}
		})
	}
}

func TestFramesEqual(t *testing.T) {
	pal := color.Palette{color.RGBA{A: 255}, color.RGBA{R: 255, A: 255}}
	base := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)

	same := image.NewPaletted(base.Bounds(), pal)
	if !framesEqual(base, same) {
		t.Error("identical images reported as different")
	}

	changed := image.NewPaletted(base.Bounds(), pal)
	changed.SetColorIndex(2, 3, 1)
	if framesEqual(base, changed) {
		t.Error("images with a differing pixel reported as equal")
	}

	other := image.NewPaletted(image.Rect(0, 0, 5, 4), pal)
	if framesEqual(base, other) {
		t.Error("images with different bounds reported as equal")
	}
}

func TestComputeDelta(t *testing.T) {
	pal := color.Palette{color.RGBA{}, color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}}
	prev := image.NewPaletted(image.Rect(0, 0, 3, 1), pal)
	curr := image.NewPaletted(image.Rect(0, 0, 3, 1), pal)
	prev.Pix = []uint8{1, 1, 1}
	curr.Pix = []uint8{1, 2, 1}

	delta := computeDelta(prev, curr, 0)
	if want := []uint8{0, 2, 0}; !bytes.Equal(delta.Pix, want) {
		t.Errorf("delta pixels = %v, want %v (unchanged -> transparent index)", delta.Pix, want)
	}
}

func paletteContains(p color.Palette, c color.RGBA) bool {
	for _, pc := range p {
		if pc == color.Color(c) {
			return true
		}
	}
	return false
}

func newPaletteRenderer(t *testing.T) (*Renderer, *renderer.Config) {
	t.Helper()
	cfg := renderer.DefaultConfig()
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r, cfg
}

func TestRenderer_buildPalette_IncludesTransparentThemeAndCatalogColors(t *testing.T) {
	r, cfg := newPaletteRenderer(t)
	rec := gifTestRecording()
	pal := r.buildPalette(rec)
	if pal[0] != color.Color(color.RGBA{}) {
		t.Errorf("first entry = %v, want transparent", pal[0])
	}
	for _, c := range []color.RGBA{
		cfg.Theme.Background, cfg.Theme.WindowBackground, cfg.Theme.Foreground,
		cfg.Theme.WindowButtons[0], rec.Colors.DefaultForeground(), rec.Colors.DefaultBackground(),
	} {
		if !paletteContains(pal, c) {
			t.Errorf("palette missing %v", c)
		}
	}
}

func TestRenderer_buildPalette_CappedAt256(t *testing.T) {
	r, cfg := newPaletteRenderer(t)
	rec := gifTestRecording()
	for i := range 300 {
		rec.Colors.Register(termcolor.FromRGB(uint8(i), uint8(i>>1), uint8(i%7)), &cfg.Theme.Palette)
	}
	if got := len(r.buildPalette(rec)); got != 256 {
		t.Errorf("palette size = %d, want 256", got)
	}
}

func TestRenderer_buildPalette_IsDeterministic(t *testing.T) {
	r, cfg := newPaletteRenderer(t)
	rec := gifTestRecording()
	for i := range 40 {
		rec.Colors.Register(termcolor.FromRGB(uint8(i*5), uint8(255-i), uint8(i*3)), &cfg.Theme.Palette)
	}
	first := r.buildPalette(rec)
	for range 20 {
		if got := r.buildPalette(rec); !slices.Equal(got, first) {
			t.Fatal("palette differs between calls")
		}
	}
}

func TestRenderer_buildPalette_HasNoDuplicates(t *testing.T) {
	r, cfg := newPaletteRenderer(t)
	rec := gifTestRecording()
	// A catalog colour equal to a theme colour must not add a second entry.
	fg := cfg.Theme.Foreground
	rec.Colors.Register(termcolor.FromRGB(fg.R, fg.G, fg.B), &cfg.Theme.Palette)
	seen := map[color.Color]bool{}
	for _, c := range r.buildPalette(rec) {
		if seen[c] {
			t.Fatalf("duplicate palette entry %v", c)
		}
		seen[c] = true
	}
}

func TestRenderer_buildPalette_OverflowKeepsThemeAndDropsNewestColors(t *testing.T) {
	r, cfg := newPaletteRenderer(t)
	rec := gifTestRecording()
	first := rec.Colors.Register(termcolor.FromRGB(0, 0, 77), &cfg.Theme.Palette)
	var last termcolor.ID
	for i := 1; i < 400; i++ {
		last = rec.Colors.Register(termcolor.FromRGB(uint8(i), uint8(i>>8), 77), &cfg.Theme.Palette)
	}

	pal := r.buildPalette(rec)
	for _, c := range []color.RGBA{
		cfg.Theme.Background, cfg.Theme.WindowBackground, cfg.Theme.Foreground,
		cfg.Theme.WindowButtons[0], cfg.Theme.WindowButtons[2],
	} {
		if !paletteContains(pal, c) {
			t.Errorf("theme colour %v dropped on overflow", c)
		}
	}
	if !paletteContains(pal, rec.Colors.Resolved(first)) {
		t.Error("the first colour used by the recording was dropped")
	}
	if paletteContains(pal, rec.Colors.Resolved(last)) {
		t.Error("the most recently introduced colour survived while 256 were already taken")
	}
}
