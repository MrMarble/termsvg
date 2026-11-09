package export

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/mrmarble/termsvg/internal/ir"
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

func TestWriteOutputFileClosesDestinationAndJoinsErrors(t *testing.T) {
	renderErr := errors.New("render failed")
	closeErr := errors.New("close failed")
	tests := []struct {
		name    string
		render  error
		wantErr []error
	}{
		{name: "close error", wantErr: []error{closeErr}},
		{name: "render and close errors", render: renderErr, wantErr: []error{renderErr, closeErr}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &closeErrorWriter{err: closeErr}
			rdr := rendererFunc(func(w io.Writer) error {
				if tt.render != nil {
					return tt.render
				}
				_, err := io.WriteString(w, "output")
				return err
			})

			err := writeOutputFile(context.Background(), rdr, nil, dst)
			for _, want := range tt.wantErr {
				if !errors.Is(err, want) {
					t.Errorf("writeOutputFile() error = %v, want it to wrap %v", err, want)
				}
			}
			if !dst.closed {
				t.Error("writeOutputFile() did not close destination")
			}
		})
	}
}

func (w *closeErrorWriter) Close() error {
	w.closed = true
	return w.err
}
