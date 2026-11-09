package svg

import "testing"

func TestLoopCountSpellings(t *testing.T) {
	for _, test := range []struct {
		loops     loopCount
		css, smil string
		infinite  bool
	}{
		{-2, "infinite", "indefinite", true},
		{-1, "1", "1", false},
		{0, "infinite", "indefinite", true},
		{1, "1", "1", false},
		{3, "3", "3", false},
	} {
		if test.loops.css() != test.css || test.loops.smil() != test.smil || test.loops.infinite() != test.infinite {
			t.Errorf("loopCount(%d) = %q, %q, %t; want %q, %q, %t", test.loops,
				test.loops.css(), test.loops.smil(), test.loops.infinite(), test.css, test.smil, test.infinite)
		}
	}
}
