package svg

import (
	"bytes"
	"context"
	"image/color"
	"io"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/renderer"
	"github.com/mrmarble/termsvg/internal/svgoutput"
)

func TestAutoLayoutExportsSmallestMinifiedCandidate(t *testing.T) {
	rec := &ir.Recording{
		Width: 20, Height: 3, Duration: 2 * time.Second,
		Colors: termcolor.NewCatalog(color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{A: 255}),
		Frames: []ir.Frame{
			{Rows: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "counter  0", EndCol: 10}}},
				{Y: 2, Runs: []ir.TextRun{{Text: "status  0", StartCol: 10, EndCol: 19}}},
			}},
			{Time: time.Second, Rows: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "counter  1", EndCol: 10}}},
				{Y: 2, Runs: []ir.TextRun{{Text: "status  1", StartCol: 10, EndCol: 19}}},
			}},
			{Time: 2 * time.Second, Rows: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "counter  2", EndCol: 10}}},
				{Y: 2, Runs: []ir.TextRun{{Text: "status  2", StartCol: 10, EndCol: 19}}},
			}},
		},
	}
	config := renderer.DefaultConfig()
	config.Minify = true
	config.ShowCursor = false
	sizes := make(map[LayoutMode]int)
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions, LayoutAuto} {
		var out bytes.Buffer
		rdr := New(config, WithLayout(layout))
		err := svgoutput.Write(&out, func(w io.Writer) error { return rdr.Render(context.Background(), rec, w) })
		if err != nil {
			t.Fatalf("%s export error = %v", layout, err)
		}
		sizes[layout] = out.Len()
		measured, err := rdr.MeasureCandidate(context.Background(), rec)
		if err != nil {
			t.Fatalf("%s MeasureCandidate() error = %v", layout, err)
		}
		if measured.FinalBytes != int64(out.Len()) {
			t.Fatalf("%s measured bytes = %d, exported bytes = %d", layout, measured.FinalBytes, out.Len())
		}
	}
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions} {
		if sizes[LayoutAuto] > sizes[layout] {
			t.Fatalf("auto bytes = %d, %s bytes = %d", sizes[LayoutAuto], layout, sizes[layout])
		}
	}
}
