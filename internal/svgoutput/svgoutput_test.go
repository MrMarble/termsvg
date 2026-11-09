package svgoutput

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestWrite_MinifiesAndRestoresSpaces(t *testing.T) {
	var out bytes.Buffer
	err := Write(&out, func(w io.Writer) error {
		_, err := io.WriteString(w, "<svg xmlns=\"http://www.w3.org/2000/svg\">\n  <text>a  b</text>\n</svg>")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "\n") {
		t.Errorf("output not minified: %q", got)
	}
	// Protected NBSPs survive minification (a plain space run would be
	// collapsed) and are restored as regular spaces afterwards.
	if !strings.Contains(got, "<text>a"+"  "+"b</text>") {
		t.Errorf("space run not preserved: %q", got)
	}
}

func TestNBSPWriter(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		want   string
	}{
		{name: "plain", chunks: []string{"abc"}, want: "abc"},
		{name: "nbsp becomes space", chunks: []string{"a b"}, want: "a b"},
		{name: "split across writes", chunks: []string{"a\xc2", "\xa0b"}, want: "a b"},
		{name: "other c2 sequence preserved", chunks: []string{"\xc2\xa9"}, want: "©"},
		{name: "trailing c2 flushed on close", chunks: []string{"a\xc2"}, want: "a\xc2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			w := newNBSPWriter(&out)
			for _, c := range tt.chunks {
				n, err := w.Write([]byte(c))
				if err != nil || n != len(c) {
					t.Fatalf("Write(%q) = %d, %v", c, n, err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if out.String() != tt.want {
				t.Errorf("got %q, want %q", out.String(), tt.want)
			}
		})
	}
}

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

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }

func (w *countingErrorWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, w.err
}

func TestWrite_MinifiesValidSVGAndRestoresSpaces(t *testing.T) {
	var dst bytes.Buffer
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"> <text>a  </text> </svg>`)
		return err
	})

	if err := Write(&dst, rdr); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if strings.Contains(dst.String(), " ") {
		t.Fatalf("Write() retained NBSP: %q", dst.String())
	}
	if !strings.Contains(dst.String(), ">a  </text>") {
		t.Fatalf("Write() lost terminal spaces: %q", dst.String())
	}
	if err := xml.Unmarshal(dst.Bytes(), new(any)); err != nil {
		t.Fatalf("Write() produced invalid XML: %v\n%s", err, dst.String())
	}
}

func TestWrite_PropagatesDestinationError(t *testing.T) {
	wantErr := errors.New("destination failed")
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w,
			`<svg xmlns="http://www.w3.org/2000/svg"><text>`+strings.Repeat("x", 8192)+`</text></svg>`)
		return err
	})

	err := Write(errorWriter{err: wantErr}, rdr)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write() error = %v, want %v", err, wantErr)
	}
}

func TestWrite_PropagatesBufferedFlushError(t *testing.T) {
	wantErr := errors.New("flush failed")
	dst := &countingErrorWriter{err: wantErr}
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"/>`)
		return err
	})

	err := Write(dst, rdr)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write() error = %v, want %v", err, wantErr)
	}
	if dst.writes != 1 {
		t.Fatalf("destination writes = %d, want 1 flush write", dst.writes)
	}
}

func TestWrite_PropagatesRenderError(t *testing.T) {
	wantErr := errors.New("render failed")
	rdr := rendererFunc(func(io.Writer) error { return wantErr })

	err := Write(io.Discard, rdr)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write() error = %v, want %v", err, wantErr)
	}
}

func TestWrite_StreamsLargeSVG(t *testing.T) {
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

	if err := Write(io.Discard, rdr); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func TestNBSPWriterPropagatesShortWrite(t *testing.T) {
	dst := &scriptedWriter{writes: []writeResult{{0, nil}, {1, nil}}}
	w := newNBSPWriter(dst)

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
	w := newNBSPWriter(dst)

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
	w := newNBSPWriter(dst)

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
	w := newNBSPWriter(dst)

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
	w := newNBSPWriter(dst)

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
	w := newNBSPWriter(&dst)

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
	w := newNBSPWriter(dst)

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
	w := newNBSPWriter(dst)

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
