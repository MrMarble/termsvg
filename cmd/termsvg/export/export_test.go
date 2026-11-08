package export

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"image/color"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/internal/svgoutput"
	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
	"github.com/mrmarble/termsvg/pkg/renderer/svg"
)

type rendererFunc func(io.Writer) error

type writeResult struct {
	n   int
	err error
}

type scriptedWriter struct {
	writes []writeResult
	output bytes.Buffer
}

type errorWriter struct{ err error }

type countingErrorWriter struct {
	err    error
	writes int
}

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

func TestWriteOutputMinifiesValidSVGAndRestoresSpaces(t *testing.T) {
	var dst bytes.Buffer
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"> <text>a  </text> </svg>`)
		return err
	})

	if err := writeOutput(context.Background(), rdr, nil, &dst, true); err != nil {
		t.Fatalf("writeOutput() error = %v", err)
	}
	if strings.Contains(dst.String(), " ") {
		t.Fatalf("writeOutput() retained NBSP: %q", dst.String())
	}
	if !strings.Contains(dst.String(), ">a  </text>") {
		t.Fatalf("writeOutput() lost terminal spaces: %q", dst.String())
	}
	if err := xml.Unmarshal(dst.Bytes(), new(any)); err != nil {
		t.Fatalf("writeOutput() produced invalid XML: %v\n%s", err, dst.String())
	}
}

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
		rdr := svg.New(config, svg.WithLayout(layout))
		if err := writeOutput(context.Background(), rdr, rec, &out, true); err != nil {
			t.Fatalf("%s writeOutput() error = %v", layout, err)
		}
		outputs[layout] = out.Bytes()
		measured, err := rdr.MeasureCandidate(context.Background(), rec)
		if err != nil {
			t.Fatalf("%s MeasureCandidate() error = %v", layout, err)
		}
		if measured.FinalBytes != int64(out.Len()) {
			t.Fatalf("%s measured bytes = %d, writeOutput bytes = %d", layout, measured.FinalBytes, out.Len())
		}
	}
	for _, layout := range []svg.LayoutMode{svg.LayoutFrames, svg.LayoutBands, svg.LayoutRegions} {
		if len(outputs[svg.LayoutAuto]) > len(outputs[layout]) {
			t.Fatalf("auto bytes = %d, %s bytes = %d", len(outputs[svg.LayoutAuto]), layout, len(outputs[layout]))
		}
	}
}

func TestNBSPWriterHandlesSplitSequence(t *testing.T) {
	var dst bytes.Buffer
	w := svgoutput.NewNBSPWriter(&dst)

	for _, chunk := range [][]byte{{'a', 0xc2}, {0xa0, 'b'}, {0xc2}, {'x'}} {
		if _, err := w.Write(chunk); err != nil {
			t.Fatalf("Write(%q) error = %v", chunk, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got, want := dst.String(), "a b\xc2x"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestWriteOutputPropagatesDestinationError(t *testing.T) {
	wantErr := errors.New("destination failed")
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w,
			`<svg xmlns="http://www.w3.org/2000/svg"><text>`+strings.Repeat("x", 8192)+`</text></svg>`)
		return err
	})

	err := writeOutput(context.Background(), rdr, nil, errorWriter{err: wantErr}, true)
	if !errors.Is(err, wantErr) {
		t.Fatalf("writeOutput() error = %v, want %v", err, wantErr)
	}
}

func TestWriteOutputPropagatesBufferedFlushError(t *testing.T) {
	wantErr := errors.New("flush failed")
	dst := &countingErrorWriter{err: wantErr}
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"/>`)
		return err
	})

	err := writeOutput(context.Background(), rdr, nil, dst, true)
	if !errors.Is(err, wantErr) {
		t.Fatalf("writeOutput() error = %v, want %v", err, wantErr)
	}
	if dst.writes != 1 {
		t.Fatalf("destination writes = %d, want 1 flush write", dst.writes)
	}
}

func TestWriteOutputPropagatesRenderError(t *testing.T) {
	wantErr := errors.New("render failed")
	rdr := rendererFunc(func(io.Writer) error { return wantErr })

	err := writeOutput(context.Background(), rdr, nil, io.Discard, true)
	if !errors.Is(err, wantErr) {
		t.Fatalf("writeOutput() error = %v, want %v", err, wantErr)
	}
}

func TestWriteOutputStreamsLargeSVG(t *testing.T) {
	wantErr := errors.New("renderer received whole-file buffer")
	rdr := rendererFunc(func(w io.Writer) error {
		if _, ok := w.(*bytes.Buffer); ok {
			return wantErr
		}
		if _, err := io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg">`); err != nil {
			return err
		}
		for range 100_000 {
			if _, err := io.WriteString(w, `<text>x</text>`); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</svg>`)
		return err
	})

	if err := writeOutput(context.Background(), rdr, nil, io.Discard, true); err != nil {
		t.Fatalf("writeOutput() error = %v", err)
	}
}

func TestNBSPWriterPropagatesShortWrite(t *testing.T) {
	dst := &scriptedWriter{writes: []writeResult{{0, nil}, {1, nil}}}
	w := svgoutput.NewNBSPWriter(dst)

	if n, err := w.Write([]byte("x")); n != 0 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Write() = (%d, %v), want (0, %v)", n, err, io.ErrShortWrite)
	}
	if n, err := w.Write([]byte("x")); n != 1 || err != nil {
		t.Fatalf("retry Write() = (%d, %v), want (1, nil)", n, err)
	}
	if got := dst.output.String(); got != "x" {
		t.Fatalf("output = %q, want %q", got, "x")
	}
}

func TestNBSPWriterCountsPartiallyWrittenInput(t *testing.T) {
	wantErr := errors.New("write failed")
	dst := &scriptedWriter{writes: []writeResult{{1, wantErr}, {1, nil}}}
	w := svgoutput.NewNBSPWriter(dst)

	if n, err := w.Write([]byte("ab")); n != 1 || !errors.Is(err, wantErr) {
		t.Fatalf("Write() = (%d, %v), want (1, %v)", n, err, wantErr)
	}
	if n, err := w.Write([]byte("b")); n != 1 || err != nil {
		t.Fatalf("retry Write() = (%d, %v), want (1, nil)", n, err)
	}
	if got := dst.output.String(); got != "ab" {
		t.Fatalf("output = %q, want %q", got, "ab")
	}
}

func TestNBSPWriterDoesNotDuplicateCompletedSplitSequence(t *testing.T) {
	wantErr := errors.New("write failed")
	dst := &scriptedWriter{writes: []writeResult{{1, wantErr}}}
	w := svgoutput.NewNBSPWriter(dst)

	if n, err := w.Write([]byte{0xc2}); n != 1 || err != nil {
		t.Fatalf("first Write() = (%d, %v), want (1, nil)", n, err)
	}
	if n, err := w.Write([]byte{0xa0}); n != 1 || !errors.Is(err, wantErr) {
		t.Fatalf("second Write() = (%d, %v), want (1, %v)", n, err, wantErr)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := dst.output.String(); got != " " {
		t.Fatalf("output = %q, want one space", got)
	}
}

func TestNBSPWriterRetriesPendingC2WithoutLosingNextByte(t *testing.T) {
	wantErr := errors.New("write failed")
	dst := &scriptedWriter{writes: []writeResult{{1, wantErr}, {1, nil}}}
	w := svgoutput.NewNBSPWriter(dst)

	if n, err := w.Write([]byte{0xc2}); n != 1 || err != nil {
		t.Fatalf("first Write() = (%d, %v), want (1, nil)", n, err)
	}
	if n, err := w.Write([]byte{'x'}); n != 0 || !errors.Is(err, wantErr) {
		t.Fatalf("second Write() = (%d, %v), want (0, %v)", n, err, wantErr)
	}
	if n, err := w.Write([]byte{'x'}); n != 1 || err != nil {
		t.Fatalf("retry Write() = (%d, %v), want (1, nil)", n, err)
	}
	if got := dst.output.String(); got != "\xc2x" {
		t.Fatalf("output = %q, want %q", got, "\xc2x")
	}
}

func TestNBSPWriterRetriesPendingC2OnClose(t *testing.T) {
	wantErr := errors.New("write failed")
	dst := &scriptedWriter{writes: []writeResult{{0, wantErr}, {1, nil}}}
	w := svgoutput.NewNBSPWriter(dst)

	if n, err := w.Write([]byte{0xc2}); n != 1 || err != nil {
		t.Fatalf("Write() = (%d, %v), want (1, nil)", n, err)
	}
	if err := w.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("Close() error = %v, want %v", err, wantErr)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("retry Close() error = %v", err)
	}
	if got := dst.output.String(); got != "\xc2" {
		t.Fatalf("output = %q, want %q", got, "\xc2")
	}
}

func TestNBSPWriterPreservesStandaloneA0(t *testing.T) {
	var dst bytes.Buffer
	w := svgoutput.NewNBSPWriter(&dst)

	if n, err := w.Write([]byte{0xa0}); n != 1 || err != nil {
		t.Fatalf("Write() = (%d, %v), want (1, nil)", n, err)
	}
	if got := dst.Bytes(); !bytes.Equal(got, []byte{0xa0}) {
		t.Fatalf("output = %x, want a0", got)
	}
}

func TestNBSPWriterCountsCompletedPendingByteOnError(t *testing.T) {
	wantErr := errors.New("write failed")
	dst := &scriptedWriter{writes: []writeResult{{1, wantErr}}}
	w := svgoutput.NewNBSPWriter(dst)

	if n, err := w.Write([]byte{0xc2, 'x'}); n != 1 || !errors.Is(err, wantErr) {
		t.Fatalf("Write() = (%d, %v), want (1, %v)", n, err, wantErr)
	}
	if n, err := w.Write([]byte{'x'}); n != 1 || err != nil {
		t.Fatalf("retry Write() = (%d, %v), want (1, nil)", n, err)
	}
	if got := dst.output.String(); got != "\xc2x" {
		t.Fatalf("output = %q, want %q", got, "\xc2x")
	}
}

func (w *scriptedWriter) Write(p []byte) (int, error) {
	if len(w.writes) == 0 {
		return w.output.Write(p)
	}
	result := w.writes[0]
	w.writes = w.writes[1:]
	if result.n > len(p) {
		result.n = len(p)
	}
	_, _ = w.output.Write(p[:result.n])
	return result.n, result.err
}

func TestNBSPWriterCloseCountsCompletedByteOnError(t *testing.T) {
	wantErr := errors.New("write failed")
	dst := &scriptedWriter{writes: []writeResult{{1, wantErr}}}
	w := svgoutput.NewNBSPWriter(dst)

	_, _ = w.Write([]byte{0xc2})
	if err := w.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("Close() error = %v, want %v", err, wantErr)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("retry Close() error = %v", err)
	}
	if got := dst.output.String(); got != "\xc2" {
		t.Fatalf("output = %q, want %q", got, "\xc2")
	}
}

func TestWriteOutputLeavesNonMinifiedBytesUnchanged(t *testing.T) {
	want := []byte(" <svg>\n\t<text>a  b</text>\n</svg> ")
	var dst bytes.Buffer
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := w.Write(want)
		return err
	})

	if err := writeOutput(context.Background(), rdr, nil, &dst, false); err != nil {
		t.Fatalf("writeOutput() error = %v", err)
	}
	if !bytes.Equal(dst.Bytes(), want) {
		t.Fatalf("writeOutput() = %q, want %q", dst.Bytes(), want)
	}
}

func TestWriteOutputFilePropagatesCloseError(t *testing.T) {
	wantErr := errors.New("close failed")
	dst := &closeErrorWriter{err: wantErr}
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, "output")
		return err
	})

	err := writeOutputFile(context.Background(), rdr, nil, dst, false)
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

	err := writeOutputFile(context.Background(), rdr, nil, dst, false)
	if !errors.Is(err, renderErr) || !errors.Is(err, closeErr) {
		t.Fatalf("writeOutputFile() error = %v, want both %v and %v", err, renderErr, closeErr)
	}
	if !dst.closed {
		t.Fatal("writeOutputFile() did not close destination")
	}
}

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }

func (w *countingErrorWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, w.err
}

func (w *closeErrorWriter) Close() error {
	w.closed = true
	return w.err
}
