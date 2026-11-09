package gif

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
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

func TestRenderer_Render_DebugLogsSummary(t *testing.T) {
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

	if want := "[GIF] 2 frames"; !strings.Contains(logs.String(), want) {
		t.Errorf("debug log = %q, want it to contain %q", logs.String(), want)
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

func decodeRender(t *testing.T, r *Renderer, rec *ir.Recording) *gif.GIF {
	t.Helper()
	var buf bytes.Buffer
	if err := r.Render(context.Background(), rec, &buf); err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatal(err)
	}

	return g
}

func TestRenderer_Render_RaisesShortDelaysToTheBrowserMinimum(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	rec := gifTestRecording(
		textFrame("a", 10*time.Millisecond),
		textFrame("b", 0),
		textFrame("c", 250*time.Millisecond),
		textFrame("d", 0),
	)

	g := decodeRender(t, r, rec)

	// Browsers show delays under 20ms for 100ms, so those are raised to 20ms
	// (2 units of 10ms). The last frame is held instead.
	if want := []int{2, 2, 25, 200}; !slices.Equal(g.Delay, want) {
		t.Errorf("delays = %v, want %v", g.Delay, want)
	}
}

func TestRenderer_Render_FramesPaintedOverEachOtherReproduceTheRecording(t *testing.T) {
	r, err := New(renderer.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	rec := gifTestRecording(
		textFrame("first", 100*time.Millisecond),
		textFrame("first and more", 100*time.Millisecond),
		textFrame("second", 100*time.Millisecond),
	)
	want, err := r.rasterizer.RasterizeWithPalette(rec, r.buildPalette(rec))
	if err != nil {
		t.Fatal(err)
	}

	g := decodeRender(t, r, rec)

	if len(g.Image) != len(want) {
		t.Fatalf("images = %d, want %d", len(g.Image), len(want))
	}
	canvas := image.NewRGBA(g.Image[0].Bounds())
	for i, frame := range g.Image {
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		for y := 0; y < canvas.Bounds().Dy(); y++ {
			for x := 0; x < canvas.Bounds().Dx(); x++ {
				if got, w := canvas.RGBAAt(x, y), want[i].Image.At(x, y); color.Color(got) != w {
					t.Fatalf("frame %d pixel (%d,%d) = %v, want %v", i, x, y, got, w)
				}
			}
		}
	}
}

func decodedPalette(t *testing.T, r *Renderer, rec *ir.Recording) color.Palette {
	t.Helper()
	return decodeRender(t, r, rec).Image[0].Palette
}

func paletteContains(p color.Palette, c color.RGBA) bool {
	return slices.Contains(p, color.Color(c))
}

func TestRenderer_Render_PaletteHoldsThemeAndRecordingColors(t *testing.T) {
	cfg := renderer.DefaultConfig()
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rec := gifTestRecording(textFrame("a", 50*time.Millisecond))
	used := rec.Colors.Register(termcolor.FromRGB(1, 2, 3), &cfg.Theme.Palette)

	pal := decodedPalette(t, r, rec)

	for _, c := range []color.RGBA{
		cfg.Theme.Background, cfg.Theme.WindowBackground, cfg.Theme.Foreground,
		cfg.Theme.WindowButtons[0], rec.Colors.DefaultForeground(), rec.Colors.DefaultBackground(),
		rec.Colors.Resolved(used),
	} {
		if !paletteContains(pal, c) {
			t.Errorf("palette missing %v", c)
		}
	}
}

func TestRenderer_Render_PaletteOverflowKeepsThemeAndDropsNewestColors(t *testing.T) {
	cfg := renderer.DefaultConfig()
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rec := gifTestRecording(textFrame("a", 50*time.Millisecond))
	first := rec.Colors.Register(termcolor.FromRGB(0, 0, 77), &cfg.Theme.Palette)
	var last termcolor.ID
	for i := 1; i < 400; i++ {
		last = rec.Colors.Register(termcolor.FromRGB(uint8(i), uint8(i>>8), 77), &cfg.Theme.Palette)
	}

	pal := decodedPalette(t, r, rec)

	if len(pal) > 256 {
		t.Errorf("palette has %d colors, a GIF holds 256", len(pal))
	}
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

func TestRenderer_Render_IsDeterministic(t *testing.T) {
	cfg := renderer.DefaultConfig()
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rec := gifTestRecording(textFrame("a", 50*time.Millisecond), textFrame("b", 50*time.Millisecond))
	for i := range 300 {
		rec.Colors.Register(termcolor.FromRGB(uint8(i), uint8(255-i), uint8(i*3)), &cfg.Theme.Palette)
	}

	var first, second bytes.Buffer
	for _, buf := range []*bytes.Buffer{&first, &second} {
		if err := r.Render(context.Background(), rec, buf); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Error("rendering the same recording twice gave different GIFs")
	}
}
