package css_test

import (
	"testing"

	"github.com/mrmarble/termsvg/internal/testutils"
	"github.com/mrmarble/termsvg/pkg/css"
)

func TestRules(t *testing.T) {
	tests := map[string]struct {
		input  css.Rules
		output string
	}{
		"Single rule": {css.Rules{"transform": "translate(10)"}, "transform:translate(10)"},
		"Multiple rule": {css.Rules{
			"transform":                 "translate(10)",
			"animation-iteration-count": "infinite",
		}, "animation-iteration-count:infinite;transform:translate(10)"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			testutils.Diff(t, test.input.String(), test.output)
		})
	}
}

func TestBlock(t *testing.T) {
	tests := map[string]struct {
		input  css.Block
		output string
	}{
		"Single rule": {css.Block{".class", css.Rules{"transform": "translate(10)"}}, ".class{transform:translate(10)}"},
		"Multiple rule": {css.Block{
			".class",
			css.Rules{
				"transform":                 "translate(10)",
				"animation-iteration-count": "infinite",
			},
		}, ".class{animation-iteration-count:infinite;transform:translate(10)}"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			testutils.Diff(t, test.input.String(), test.output)
		})
	}
}
