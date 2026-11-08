package svgoutput

import (
	"bufio"
	"errors"
	"io"

	"github.com/tdewolff/minify/v2"
	msvg "github.com/tdewolff/minify/v2/svg"
)

type nbspWriter struct {
	w       io.Writer
	pending bool
}

// Write streams SVG through the production minifier and restores protected spaces.
func Write(dst io.Writer, render func(io.Writer) error) error {
	buf := bufio.NewWriter(dst)
	spaces := NewNBSPWriter(buf)
	m := minify.New()
	m.AddFunc("image/svg+xml", msvg.Minify)
	minified := m.Writer("image/svg+xml", spaces)
	return errors.Join(render(minified), minified.Close(), spaces.Close(), buf.Flush())
}

// NewNBSPWriter restores UTF-8 non-breaking spaces after SVG minification.
func NewNBSPWriter(w io.Writer) io.WriteCloser { return &nbspWriter{w: w} }

func (w *nbspWriter) Write(p []byte) (int, error) {
	accepted := 0
	for _, b := range p {
		if w.pending {
			out := byte(0xc2)
			if b == 0xa0 {
				out = ' '
			}
			written, err := w.writeByte(out)
			if written {
				w.pending = false
				if b == 0xa0 {
					accepted++
				}
			}
			if err != nil {
				return accepted, err
			}
			if b == 0xa0 {
				continue
			}
		}
		if b == 0xc2 {
			w.pending = true
			accepted++
			continue
		}
		written, err := w.writeByte(b)
		if written {
			accepted++
		}
		if err != nil {
			return accepted, err
		}
	}
	return accepted, nil
}

func (w *nbspWriter) writeByte(b byte) (bool, error) {
	n, err := w.w.Write([]byte{b})
	if err == nil && n != 1 {
		err = io.ErrShortWrite
	}
	return n == 1, err
}

func (w *nbspWriter) Close() error {
	if !w.pending {
		return nil
	}
	written, err := w.writeByte(0xc2)
	if written {
		w.pending = false
	}
	return err
}
