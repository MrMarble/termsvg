package layout

import "testing"

func TestSizes(t *testing.T) {
	if got := ContentOffsetY(true); got != 40 {
		t.Errorf("ContentOffsetY(true) = %d, want 40", got)
	}
	if got := ContentOffsetY(false); got != 20 {
		t.Errorf("ContentOffsetY(false) = %d, want 20", got)
	}
	if got := PaddedWidth(960); got != 1000 {
		t.Errorf("PaddedWidth(960) = %d, want 1000", got)
	}
	if got := PaddedHeight(250, true); got != 310 {
		t.Errorf("PaddedHeight(250, true) = %d, want 310", got)
	}
	if got := PaddedHeight(250, false); got != 290 {
		t.Errorf("PaddedHeight(250, false) = %d, want 290", got)
	}
}
