package css_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mrmarble/termsvg/pkg/css"
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		input  css.CSS
		output string
	}{
		"Single rule": {css.CSS{"transform": "translate(10)"}, "transform:translate(10)"},
		"Multiple rule": {css.CSS{
			"transform":                 "translate(10)",
			"animation-iteration-count": "infinite",
		}, "animation-iteration-count:infinite;transform:translate(10)"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			diff(t, test.input.Compile(), test.output)
		})
	}
}

func diff(t *testing.T, x interface{}, y interface{}) {
	t.Helper()

	diff := cmp.Diff(x, y)
	if diff != "" {
		t.Fatalf(diff)
	}
}
