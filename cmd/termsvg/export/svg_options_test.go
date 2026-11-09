package export

import (
	"testing"

	"github.com/mrmarble/termsvg/internal/renderer/svg"
)

// svg.Options.Validate has its own tests; these cover how the flags map onto
// the options and how they interact with the output format.
func TestNormalizedSVGOptions(t *testing.T) {
	tests := []struct {
		name    string
		cmd     Cmd
		format  string
		want    svg.Options
		wantErr bool
	}{
		{name: "zero value defaults", cmd: Cmd{}, format: "svg", want: svg.DefaultOptions()},
		{
			name:   "every flag is mapped case-insensitively",
			cmd:    Cmd{SVGLayout: "REGIONS", SVGAnimation: "SMIL", SVGFrameSwitch: "HREF", SVGStyle: "AUTO", SVGPrimitives: "RECT-TRACKS", SVGMaxFPS: 30},
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
		{name: "invalid combination is rejected", cmd: Cmd{SVGLayout: "regions", SVGPrimitives: "rect-tracks"}, format: "svg", wantErr: true},
		{name: "non SVG accepts defaults", cmd: Cmd{SVGLayout: "frames", SVGAnimation: "css"}, format: "webm", want: svg.DefaultOptions()},
		{name: "non SVG rejects layout", cmd: Cmd{SVGLayout: "bands"}, format: "gif", wantErr: true},
		{name: "non SVG rejects style", cmd: Cmd{SVGStyle: "auto"}, format: "webm", wantErr: true},
		{name: "non SVG rejects href", cmd: Cmd{SVGAnimation: "smil", SVGFrameSwitch: "href"}, format: "gif", wantErr: true},
		{name: "non SVG rejects runtime objective", cmd: Cmd{SVGLayout: "auto", SVGAutoObjective: "runtime"}, format: "gif", wantErr: true},
		{name: "SVG rejects system fonts", cmd: Cmd{SystemFonts: true}, format: "svg", wantErr: true},
		{name: "raster accepts system fonts", cmd: Cmd{SystemFonts: true}, format: "gif", want: svg.DefaultOptions()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.cmd.normalizedSVGOptions(test.format)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizedSVGOptions() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("normalizedSVGOptions() = %#v, want %#v", got, test.want)
			}
		})
	}
}
