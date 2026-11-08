package renderer

import (
	"testing"

	"github.com/mrmarble/termsvg/internal/progress"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if !c.ShowWindow || !c.ShowCursor || c.Minify || c.Debug {
		t.Errorf("unexpected flags: %+v", c)
	}
	if c.FontSize != 20 || c.LoopCount != 0 || c.FontFamily == "" {
		t.Errorf("unexpected values: %+v", c)
	}
	if c.Theme.Name != "default" {
		t.Errorf("Theme = %q", c.Theme.Name)
	}
	if DefaultConfig() == c {
		t.Error("DefaultConfig must return a fresh value")
	}
}

func TestNewRasterizer(t *testing.T) {
	c := DefaultConfig()
	ch := make(chan progress.Update, 1)
	c.ProgressCh = ch
	r, err := NewRasterizer(c)
	if err != nil || r == nil {
		t.Fatalf("NewRasterizer() = %v, %v", r, err)
	}
}
