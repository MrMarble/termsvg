package renderer

import "testing"

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
