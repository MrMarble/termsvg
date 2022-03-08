package css_test

import (
	"testing"

	"github.com/mrmarble/termsvg/internal/testutils"
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
			testutils.Diff(t, test.input.Compile(), test.output)
		})
	}
}
