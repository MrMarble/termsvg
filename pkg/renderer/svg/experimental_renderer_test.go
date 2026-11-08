package svg

import (
	"bytes"
	"context"
	stdcolor "image/color"
	"strings"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func experimentalRecording() *ir.Recording {
	catalog := termcolor.NewCatalog(
		stdcolor.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		stdcolor.RGBA{A: 0xff},
	)
	static := experimentalRow(1, "static")
	return &ir.Recording{
		Width:    10,
		Height:   4,
		Duration: 2 * time.Second,
		Colors:   catalog,
		Frames: []ir.Frame{
			{Rows: []ir.Row{experimentalRow(0, "a"), static, experimentalRow(2, "x")}, Cursor: ir.Cursor{Visible: true}},
			{
				Time:   time.Second,
				Rows:   []ir.Row{experimentalRow(0, "b"), static, experimentalRow(2, "y")},
				Cursor: ir.Cursor{Col: 1, Visible: true},
			},
			{
				Time:   2 * time.Second,
				Rows:   []ir.Row{experimentalRow(0, "b"), static, experimentalRow(2, "y")},
				Cursor: ir.Cursor{Col: 1},
			},
		},
	}
}

func renderExperimentalSVG(t *testing.T, options ...Option) string {
	t.Helper()
	config := renderer.DefaultConfig()
	config.ShowWindow = false
	var out bytes.Buffer
	if err := New(config, options...).Render(context.Background(), experimentalRecording(), &out); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return out.String()
}

func TestDefaultSVGOptionsRetainCSSFrameStrip(t *testing.T) {
	got := renderExperimentalSVG(t)
	if !strings.Contains(got, "@keyframes k{") || !strings.Contains(got, "animation:k ") {
		t.Fatalf("default output does not contain compatibility frame animation: %s", got)
	}
	if strings.Contains(got, "@keyframes b0{") || strings.Contains(got, "<animateTransform") {
		t.Fatalf("default output enabled an experimental backend: %s", got)
	}
}

func TestBandLayoutUsesIndependentBandsAndSharedTimelineCSS(t *testing.T) {
	got := renderExperimentalSVG(t, WithLayout(LayoutBands))
	if strings.Contains(got, "@keyframes k{") || strings.Contains(got, "animation:k ") {
		t.Fatalf("band output retained the global frame animation: %s", got)
	}
	if count := strings.Count(got, "@keyframes b0{"); count != 1 {
		t.Fatalf("shared band keyframes count = %d, want 1: %s", count, got)
	}
	if count := strings.Count(got, "animation:b0 "); count != 2 {
		t.Fatalf("band animation uses = %d, want 2 non-adjacent bands: %s", count, got)
	}
	if !strings.Contains(got, `<svg x="0" y="50" width="24" height="25" overflow="hidden">`) {
		t.Fatalf("row 2 band is not positioned independently: %s", got)
	}
}

func TestSMILBackendOmitsContentAndCursorCSSKeyframes(t *testing.T) {
	got := renderExperimentalSVG(t, WithAnimation(AnimationSMIL))
	if strings.Contains(got, "@keyframes k{") || strings.Contains(got, "@keyframes cursor{") ||
		strings.Contains(got, "animation:k ") || strings.Contains(got, "animation:cursor ") {
		t.Fatalf("SMIL output retained CSS timeline animation: %s", got)
	}
	if count := strings.Count(got, "<animateTransform"); count != 2 {
		t.Fatalf("animateTransform count = %d, want content and cursor: %s", count, got)
	}
	if !strings.Contains(got, `values="0;-120;-120" keyTimes="0;.5;1"`) {
		t.Fatalf("content SMIL timeline missing: %s", got)
	}
	if strings.Contains(got, `attributeName="visibility"`) {
		t.Fatalf("zero-dwell cursor endpoint was retained: %s", got)
	}
	if !strings.Contains(got, `repeatCount="indefinite"`) {
		t.Fatalf("SMIL infinite loop mapping missing: %s", got)
	}
}

func TestBandAndSMILBackendsCompose(t *testing.T) {
	got := renderExperimentalSVG(t, WithLayout(LayoutBands), WithAnimation(AnimationSMIL))
	if strings.Contains(got, "@keyframes b0{") || strings.Contains(got, "animation:b0 ") {
		t.Fatalf("band SMIL output retained CSS band animation: %s", got)
	}
	if count := strings.Count(got, "<animateTransform"); count != 3 {
		t.Fatalf("animateTransform count = %d, want two bands and cursor: %s", count, got)
	}
}

func TestStaticSMILRecordingContainsNoTimelineElements(t *testing.T) {
	rec := experimentalRecording()
	rec.Duration = 0
	rec.Frames = rec.Frames[:1]
	config := renderer.DefaultConfig()
	config.ShowCursor = false
	var out bytes.Buffer
	if err := New(config, WithAnimation(AnimationSMIL)).Render(context.Background(), rec, &out); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(out.String(), "<animate") {
		t.Fatalf("static output contains timeline elements: %s", out.String())
	}
}
