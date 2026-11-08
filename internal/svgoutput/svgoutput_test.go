package svgoutput

import (
	"bytes"
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

func TestWrite_PropagatesRenderError(t *testing.T) {
	want := errors.New("boom")
	err := Write(io.Discard, func(io.Writer) error { return want })
	if !errors.Is(err, want) {
		t.Errorf("error = %v, want %v", err, want)
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
			w := NewNBSPWriter(&out)
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

type failWriter struct{ n int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.n == 0 {
		return 0, errors.New("write failed")
	}
	f.n--
	return len(p), nil
}

type shortWriter struct{}

func (shortWriter) Write([]byte) (int, error) { return 0, nil }

func TestNBSPWriter_Errors(t *testing.T) {
	t.Run("underlying error", func(t *testing.T) {
		w := NewNBSPWriter(&failWriter{})
		if _, err := w.Write([]byte("x")); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("short write", func(t *testing.T) {
		w := NewNBSPWriter(shortWriter{})
		if _, err := w.Write([]byte("x")); !errors.Is(err, io.ErrShortWrite) {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("close flush error", func(t *testing.T) {
		w := NewNBSPWriter(&failWriter{n: 0})
		_, _ = w.Write([]byte("\xc2"))
		if err := w.Close(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("pending byte then failing write", func(t *testing.T) {
		w := NewNBSPWriter(&failWriter{})
		_, _ = w.Write([]byte("\xc2"))
		if _, err := w.Write([]byte("a")); err == nil {
			t.Error("expected error")
		}
	})
}
