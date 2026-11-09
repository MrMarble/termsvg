package raster

import (
	"image"
	"image/color"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/layout"
	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/renderer"
)

var (
	testFG   = color.RGBA{R: 192, G: 192, B: 192, A: 255}
	testBG   = color.RGBA{R: 0, G: 0, B: 0, A: 255}
	testRed  = color.RGBA{R: 255, A: 255}
	testBlue = color.RGBA{B: 255, A: 255}
)

// testPalette contains every color the tests draw with, so that nearest-color
// quantization is exact for solid fills.
func testPalette(cfg *renderer.Config) color.Palette {
	return color.Palette{
		testBG, testFG, testRed, testBlue,
		cfg.Theme.WindowBackground,
		cfg.Theme.WindowButtons[0], cfg.Theme.WindowButtons[1], cfg.Theme.WindowButtons[2],
		color.RGBA{R: 96, G: 96, B: 96, A: 255}, // dimmed foreground
		color.RGBA{A: 0},
	}
}

func indexOf(t *testing.T, p color.Palette, c color.RGBA) uint8 {
	t.Helper()
	for i, pc := range p {
		if pc == color.Color(c) {
			return uint8(i)
		}
	}
	t.Fatalf("%v not in palette", c)
	return 0
}

func newTestRasterizer(t *testing.T, mutate func(*renderer.Config)) (*Rasterizer, *renderer.Config) {
	t.Helper()
	cfg := testConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	r, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r, &cfg
}

func newCatalog() *termcolor.Catalog { return termcolor.NewCatalog(testFG, testBG) }

func recordingWith(catalog *termcolor.Catalog, frames ...ir.Frame) *ir.Recording {
	return &ir.Recording{Width: 10, Height: 4, Duration: time.Second, Frames: frames, Colors: catalog}
}

func TestRasterizeWithPalette_EmptyRecording(t *testing.T) {
	r, cfg := newTestRasterizer(t, nil)
	if _, err := r.RasterizeWithPalette(&ir.Recording{}, testPalette(cfg)); err == nil {
		t.Fatal("expected error for a recording without frames")
	}
}

func TestRasterizeWithPalette_FramesKeepOrderDelayAndSize(t *testing.T) {
	r, cfg := newTestRasterizer(t, nil)
	palette := testPalette(cfg)
	rec := recordingWith(newCatalog(),
		ir.Frame{Delay: 10 * time.Millisecond},
		ir.Frame{Delay: 20 * time.Millisecond},
		ir.Frame{Delay: 30 * time.Millisecond},
	)

	frames, err := r.RasterizeWithPalette(rec, palette)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 {
		t.Fatalf("frames = %d, want 3", len(frames))
	}
	wantBounds := image.Rect(0, 0, 160, 160) // 120x100 content, 20px padding, 40px header
	for i, f := range frames {
		want := time.Duration(10*(i+1)) * time.Millisecond
		if i == len(frames)-1 {
			want = LastFrameHold // the last frame is held so it can be read
		}
		if f.Index != i || f.Delay != want {
			t.Errorf("frame %d: index=%d delay=%v, want %v", i, f.Index, f.Delay, want)
		}
		if f.Image == nil || f.Image.Bounds() != wantBounds {
			t.Fatalf("frame %d bounds = %v, want %v", i, f.Image.Bounds(), wantBounds)
		}
		if len(f.Image.Palette) != len(palette) {
			t.Errorf("frame %d palette size = %d, want %d", i, len(f.Image.Palette), len(palette))
		}
	}
	if frames[0].Image == frames[1].Image {
		t.Error("frames must not share an image")
	}
}

func TestRasterizeWithPalette_Cursor(t *testing.T) {
	cursorAt := func(r *Rasterizer, col, row int) image.Point {
		return image.Pt(
			layout.Padding+col*layout.ColWidth+layout.ColWidth/2,
			layout.ContentOffsetY(r.config.ShowWindow)+row*layout.RowHeight+layout.RowHeight/2,
		)
	}
	tests := []struct {
		name       string
		showCursor bool
		visible    bool
		wantDrawn  bool
	}{
		{name: "visible and enabled", showCursor: true, visible: true, wantDrawn: true},
		{name: "hidden by recording", showCursor: true, visible: false},
		{name: "disabled by config", showCursor: false, visible: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, cfg := newTestRasterizer(t, func(c *renderer.Config) { c.ShowCursor = tt.showCursor })
			palette := testPalette(cfg)
			frame := ir.Frame{Cursor: ir.Cursor{Col: 3, Row: 1, Visible: tt.visible}}

			frames, err := r.RasterizeWithPalette(recordingWith(newCatalog(), frame), palette)
			if err != nil {
				t.Fatal(err)
			}
			p := cursorAt(r, 3, 1)
			got := frames[0].Image.ColorIndexAt(p.X, p.Y)
			fg, bg := indexOf(t, palette, testFG), indexOf(t, palette, cfg.Theme.WindowBackground)
			want := bg
			if tt.wantDrawn {
				want = fg
			}
			if got != want {
				t.Errorf("pixel %v index = %d, want %d", p, got, want)
			}
		})
	}
}

func TestRasterizeWithPalette_TextBackgroundAndUnderline(t *testing.T) {
	r, cfg := newTestRasterizer(t, nil)
	palette := testPalette(cfg)
	catalog := newCatalog()
	red := catalog.Register(termcolor.FromRGB(testRed.R, testRed.G, testRed.B), &cfg.Theme.Palette)

	frame := ir.Frame{Rows: []ir.Row{{Y: 1, Runs: []ir.TextRun{
		{Text: "   ", StartCol: 0, Attrs: ir.CellAttrs{BG: red}},
		{Text: "WWW", StartCol: 4, Attrs: ir.CellAttrs{Underline: true}},
		{Text: "", StartCol: 8},
	}}}}

	frames, err := r.RasterizeWithPalette(recordingWith(catalog, frame), palette)
	if err != nil {
		t.Fatal(err)
	}
	img := frames[0].Image
	rowTop := layout.ContentOffsetY(r.config.ShowWindow) + 1*layout.RowHeight
	idxRed, idxFG, idxWin := indexOf(t, palette, testRed), indexOf(t, palette, testFG),
		indexOf(t, palette, cfg.Theme.WindowBackground)

	t.Run("background fills the run", func(t *testing.T) {
		x := layout.Padding + 1*layout.ColWidth
		if got := img.ColorIndexAt(x, rowTop+2); got != idxRed {
			t.Errorf("bg pixel index = %d, want red (%d)", got, idxRed)
		}
	})
	t.Run("underline spans the run width", func(t *testing.T) {
		y := rowTop + layout.RowHeight - underlineOffset
		for x := layout.Padding + 4*layout.ColWidth; x < layout.Padding+7*layout.ColWidth; x++ {
			if got := img.ColorIndexAt(x, y); got != idxFG {
				t.Fatalf("underline pixel (%d,%d) index = %d, want fg (%d)", x, y, got, idxFG)
			}
		}
	})
	t.Run("glyphs are drawn", func(t *testing.T) {
		changed := 0
		for y := rowTop; y < rowTop+layout.RowHeight-underlineOffset-1; y++ {
			for x := layout.Padding + 4*layout.ColWidth; x < layout.Padding+7*layout.ColWidth; x++ {
				if img.ColorIndexAt(x, y) != idxWin {
					changed++
				}
			}
		}
		if changed == 0 {
			t.Error("no glyph pixels were drawn")
		}
	})
	t.Run("empty run and untouched area stay background", func(t *testing.T) {
		if got := img.ColorIndexAt(layout.Padding+8*layout.ColWidth+2, rowTop+2); got != idxWin {
			t.Errorf("index = %d, want window background (%d)", got, idxWin)
		}
	})
}

func TestRasterizeWithPalette_WithoutWindow(t *testing.T) {
	r, cfg := newTestRasterizer(t, func(c *renderer.Config) { c.ShowWindow = false })
	frames, err := r.RasterizeWithPalette(recordingWith(newCatalog(), ir.Frame{}), testPalette(cfg))
	if err != nil {
		t.Fatal(err)
	}
	want := image.Rect(0, 0, 160, 140) // 120x100 content and 20px padding, no window
	if got := frames[0].Image.Bounds(); got != want {
		t.Errorf("bounds = %v, want %v", got, want)
	}
}

func TestRasterizeWithPalette_ReportsProgress(t *testing.T) {
	ch := make(chan progress.Update, 16)
	r, cfg := newTestRasterizer(t, func(c *renderer.Config) { c.ProgressCh = ch })
	rec := recordingWith(newCatalog(), ir.Frame{}, ir.Frame{}, ir.Frame{})

	if _, err := r.RasterizeWithPalette(rec, testPalette(cfg)); err != nil {
		t.Fatal(err)
	}
	close(ch)

	var updates []progress.Update
	for u := range ch {
		updates = append(updates, u)
	}
	if len(updates) != 4 { // initial 0/3 plus one per frame
		t.Fatalf("updates = %d, want 4: %+v", len(updates), updates)
	}
	if updates[0].Current != 0 || updates[0].Total != 3 || updates[0].Phase != "Rasterizing" {
		t.Errorf("first update = %+v", updates[0])
	}
	maxCurrent := 0
	for _, u := range updates {
		maxCurrent = max(maxCurrent, u.Current)
	}
	if maxCurrent != 3 {
		t.Errorf("highest Current = %d, want 3", maxCurrent)
	}
}
