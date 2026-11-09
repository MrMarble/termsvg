package renderer

import (
	"testing"

	"github.com/mrmarble/termsvg/internal/progress"
)

func TestNewRasterizer(t *testing.T) {
	c := DefaultConfig()
	ch := make(chan progress.Update, 1)
	c.ProgressCh = ch
	r, err := NewRasterizer(c)
	if err != nil || r == nil {
		t.Fatalf("NewRasterizer() = %v, %v", r, err)
	}
}
