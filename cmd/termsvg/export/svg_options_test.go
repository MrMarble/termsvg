//nolint:lll // Compact option table rows are easier to compare.
package export

import (
	"testing"

	"github.com/mrmarble/termsvg/pkg/renderer/svg"
)

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
			name: "regions layout", cmd: Cmd{SVGLayout: "REGIONS"}, format: "svg",
			want: svg.Options{Layout: svg.LayoutRegions, Animation: svg.AnimationCSS, FrameSwitch: svg.FrameSwitchTranslate, AutoObjective: svg.AutoObjectiveSize, Style: svg.StyleLegacy, Primitives: svg.PrimitiveSnapshots},
		},
		{
			name: "scroll layout", cmd: Cmd{SVGLayout: "SCROLL"}, format: "svg",
			want: svg.Options{Layout: svg.LayoutScroll, Animation: svg.AnimationCSS, FrameSwitch: svg.FrameSwitchTranslate, AutoObjective: svg.AutoObjectiveSize, Style: svg.StyleLegacy, Primitives: svg.PrimitiveSnapshots},
		},
		{
			name:   "experimental options",
			cmd:    Cmd{SVGLayout: "BANDS", SVGAnimation: "SMIL", SVGMaxFPS: 30},
			format: "svg",
			want: svg.Options{
				Layout:        svg.LayoutBands,
				Animation:     svg.AnimationSMIL,
				FrameSwitch:   svg.FrameSwitchTranslate,
				MaxFPS:        30,
				AutoObjective: svg.AutoObjectiveSize, Style: svg.StyleLegacy, Primitives: svg.PrimitiveSnapshots,
			},
		},
		{name: "runtime objective", cmd: Cmd{SVGLayout: "AUTO", SVGAutoObjective: "RUNTIME"}, format: "svg", want: svg.Options{Layout: svg.LayoutAuto, Animation: svg.AnimationCSS, FrameSwitch: svg.FrameSwitchTranslate, AutoObjective: svg.AutoObjectiveRuntime, Style: svg.StyleLegacy, Primitives: svg.PrimitiveSnapshots}},
		{name: "auto style", cmd: Cmd{SVGStyle: "AUTO"}, format: "svg", want: svg.Options{Layout: svg.LayoutFrames, Animation: svg.AnimationCSS, FrameSwitch: svg.FrameSwitchTranslate, AutoObjective: svg.AutoObjectiveSize, Style: svg.StyleAuto, Primitives: svg.PrimitiveSnapshots}},
		{name: "rect tracks", cmd: Cmd{SVGLayout: "regions", SVGAnimation: "smil", SVGPrimitives: "RECT-TRACKS"}, format: "svg", want: svg.Options{Layout: svg.LayoutRegions, Animation: svg.AnimationSMIL, FrameSwitch: svg.FrameSwitchTranslate, AutoObjective: svg.AutoObjectiveSize, Style: svg.StyleLegacy, Primitives: svg.PrimitiveRectTracks}},
		{name: "rect tracks require SMIL", cmd: Cmd{SVGLayout: "regions", SVGPrimitives: "rect-tracks"}, format: "svg", wantErr: true},
		{name: "rect tracks reject auto", cmd: Cmd{SVGLayout: "auto", SVGAnimation: "smil", SVGPrimitives: "rect-tracks"}, format: "svg", wantErr: true},
		{name: "non SVG rejects rect tracks", cmd: Cmd{SVGLayout: "regions", SVGAnimation: "smil", SVGPrimitives: "rect-tracks"}, format: "gif", wantErr: true},
		{name: "invalid style", cmd: Cmd{SVGStyle: "compact"}, format: "svg", wantErr: true},
		{name: "non SVG rejects style", cmd: Cmd{SVGStyle: "auto"}, format: "gif", wantErr: true},
		{name: "WebM rejects style", cmd: Cmd{SVGStyle: "auto"}, format: "webm", wantErr: true},
		{name: "runtime requires auto", cmd: Cmd{SVGAutoObjective: "runtime"}, format: "svg", wantErr: true},
		{name: "invalid objective", cmd: Cmd{SVGLayout: "auto", SVGAutoObjective: "balanced"}, format: "svg", wantErr: true},
		{name: "non SVG rejects runtime", cmd: Cmd{SVGLayout: "auto", SVGAutoObjective: "runtime"}, format: "gif", wantErr: true},
		{name: "negative FPS", cmd: Cmd{SVGMaxFPS: -1}, format: "svg", wantErr: true},
		{name: "non SVG rejects non-default", cmd: Cmd{SVGLayout: "bands"}, format: "gif", wantErr: true},
		{
			name:   "non SVG accepts defaults",
			cmd:    Cmd{SVGLayout: "frames", SVGAnimation: "css"},
			format: "webm",
			want:   svg.DefaultOptions(),
		},
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
