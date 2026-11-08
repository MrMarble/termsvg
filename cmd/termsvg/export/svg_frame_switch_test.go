package export

import (
	"testing"

	"github.com/mrmarble/termsvg/pkg/renderer/svg"
)

func TestNormalizedSVGOptionsAcceptsSMILHref(t *testing.T) {
	cmd := Cmd{SVGLayout: "bands", SVGAnimation: "smil", SVGFrameSwitch: "href"}

	options, err := cmd.normalizedSVGOptions("svg")
	if err != nil {
		t.Fatalf("normalizedSVGOptions() error = %v", err)
	}
	if options.Layout != svg.LayoutBands || options.Animation != svg.AnimationSMIL ||
		options.FrameSwitch != svg.FrameSwitchHref {
		t.Fatalf("normalized options = %#v", options)
	}
}

func TestNormalizedSVGOptionsRejectsHrefForNonSVG(t *testing.T) {
	cmd := Cmd{SVGAnimation: "smil", SVGFrameSwitch: "href"}
	if _, err := cmd.normalizedSVGOptions("gif"); err == nil {
		t.Fatal("SVG href option unexpectedly accepted for GIF")
	}
}
