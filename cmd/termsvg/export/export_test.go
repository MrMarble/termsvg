package export

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"io"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/renderer"
	"github.com/mrmarble/termsvg/internal/renderer/svg"
)

type rendererFunc func(io.Writer) error

type closeErrorWriter struct {
	bytes.Buffer
	err    error
	closed bool
}

func (f rendererFunc) Render(_ context.Context, _ *ir.Recording, w io.Writer) error {
	return f(w)
}

func (rendererFunc) Format() string        { return "svg" }
func (rendererFunc) FileExtension() string { return ".svg" }

func TestWriteOutputAutoChoosesSmallestMinifiedCandidate(t *testing.T) {
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
	outputs := make(map[svg.LayoutMode][]byte)
	for _, layout := range []svg.LayoutMode{svg.LayoutFrames, svg.LayoutBands, svg.LayoutRegions, svg.LayoutAuto} {
		var out bytes.Buffer
		rdr := svg.New(config, svg.WithOptions(svg.Options{Layout: layout}))
		if err := rdr.Render(context.Background(), rec, &out); err != nil {
			t.Fatalf("%s Render() error = %v", layout, err)
		}
		outputs[layout] = out.Bytes()
	}
	for _, layout := range []svg.LayoutMode{svg.LayoutFrames, svg.LayoutBands, svg.LayoutRegions} {
		if len(outputs[svg.LayoutAuto]) > len(outputs[layout]) {
			t.Fatalf("auto bytes = %d, %s bytes = %d", len(outputs[svg.LayoutAuto]), layout, len(outputs[layout]))
		}
	}
}

func TestWriteOutputFilePropagatesCloseError(t *testing.T) {
	wantErr := errors.New("close failed")
	dst := &closeErrorWriter{err: wantErr}
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, "output")
		return err
	})

	err := writeOutputFile(context.Background(), rdr, nil, dst)
	if !errors.Is(err, wantErr) {
		t.Fatalf("writeOutputFile() error = %v, want %v", err, wantErr)
	}
	if !dst.closed {
		t.Fatal("writeOutputFile() did not close destination")
	}
}

func TestWriteOutputFileJoinsRenderAndCloseErrors(t *testing.T) {
	renderErr := errors.New("render failed")
	closeErr := errors.New("close failed")
	dst := &closeErrorWriter{err: closeErr}
	rdr := rendererFunc(func(io.Writer) error { return renderErr })

	err := writeOutputFile(context.Background(), rdr, nil, dst)
	if !errors.Is(err, renderErr) || !errors.Is(err, closeErr) {
		t.Fatalf("writeOutputFile() error = %v, want both %v and %v", err, renderErr, closeErr)
	}
	if !dst.closed {
		t.Fatal("writeOutputFile() did not close destination")
	}
}

func (w *closeErrorWriter) Close() error {
	w.closed = true
	return w.err
}
