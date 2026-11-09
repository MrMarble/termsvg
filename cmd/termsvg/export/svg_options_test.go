package export

import (
	"testing"

	"github.com/mrmarble/termsvg/internal/renderer/svg"
)

// svg.Options.Validate has its own tests; these cover how the flags map onto
// the options and how they interact with the output format.
func TestNormalizedSVGOptions(t *testing.T) {
	tests := []struct {
		name   string
		cmd    Cmd
		format string
		want   svg.Options
	}{
		{name: "zero value defaults", cmd: Cmd{}, format: "svg", want: svg.DefaultOptions()},
		{
			name: "every flag is mapped case-insensitively",
			cmd: Cmd{
				SVGLayout: "REGIONS", SVGAnimation: "SMIL", SVGFrameSwitch: "HREF",
				SVGStyle: "AUTO", SVGPrimitives: "RECT-TRACKS", SVGMaxFPS: 30,
			},
			format: "svg",
			want: svg.Options{
				Layout: svg.LayoutRegions, Animation: svg.AnimationSMIL, FrameSwitch: svg.FrameSwitchHref,
				AutoObjective: svg.AutoObjectiveSize, Style: svg.StyleAuto, Primitives: svg.PrimitiveRectTracks, MaxFPS: 30,
			},
		},
		{
			name:   "auto objective",
			cmd:    Cmd{SVGLayout: "auto", SVGAutoObjective: "RUNTIME"},
			format: "svg",
			want: svg.Options{
				Layout: svg.LayoutAuto, Animation: svg.AnimationCSS, FrameSwitch: svg.FrameSwitchTranslate,
				AutoObjective: svg.AutoObjectiveRuntime, Style: svg.StyleLegacy, Primitives: svg.PrimitiveSnapshots,
			},
		},
		{
			name:   "non SVG accepts defaults",
			cmd:    Cmd{SVGLayout: "frames", SVGAnimation: "css"},
			format: "webm",
			want:   svg.DefaultOptions(),
		},
		{name: "raster accepts system fonts", cmd: Cmd{SystemFonts: true}, format: "gif", want: svg.DefaultOptions()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.cmd.checkFormatFlags(test.format)
			if err != nil {
				t.Fatalf("checkFormatFlags() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("checkFormatFlags() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCheckFormatFlagsRejects(t *testing.T) {
	tests := []struct {
		name   string
		cmd    Cmd
		format string
	}{
		{"invalid combination", Cmd{SVGLayout: "regions", SVGPrimitives: "rect-tracks"}, "svg"},
		{"non SVG layout", Cmd{SVGLayout: "bands"}, "gif"},
		{"non SVG style", Cmd{SVGStyle: "auto"}, "webm"},
		{"non SVG href", Cmd{SVGAnimation: "smil", SVGFrameSwitch: "href"}, "gif"},
		{"non SVG runtime objective", Cmd{SVGLayout: "auto", SVGAutoObjective: "runtime"}, "gif"},
		{"system fonts for SVG", Cmd{SystemFonts: true}, "svg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.cmd.checkFormatFlags(test.format); err == nil {
				t.Fatal("checkFormatFlags() error = nil, want an error")
			}
		})
	}
}
