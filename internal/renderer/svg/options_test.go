//nolint:lll // Compact option literals make defaults easy to compare.
package svg

import (
	"testing"
)

func TestRectTracksRequireSMILLocalLayout(t *testing.T) {
	for _, options := range []Options{
		{Layout: LayoutRegions, Animation: AnimationCSS, Primitives: PrimitiveRectTracks},
		{Layout: LayoutFrames, Animation: AnimationSMIL, Primitives: PrimitiveRectTracks},
		{Layout: LayoutAuto, Animation: AnimationSMIL, Primitives: PrimitiveRectTracks},
	} {
		if err := options.Validate(); err == nil {
			t.Fatalf("Validate(%#v) succeeded", options)
		}
	}
	options := Options{Layout: LayoutRegions, Animation: AnimationSMIL, Primitives: PrimitiveRectTracks}
	if err := options.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRendererOptionsValidation(t *testing.T) {
	tests := []struct {
		name    string
		options Options
	}{
		{name: "layout", options: Options{Layout: "columns", Animation: AnimationCSS}},
		{name: "animation", options: Options{Layout: LayoutFrames, Animation: "script"}},
		{name: "objective", options: Options{Layout: LayoutAuto, Animation: AnimationCSS, AutoObjective: "balanced"}},
		{name: "style", options: Options{Layout: LayoutFrames, Animation: AnimationCSS, Style: "compact"}},
		{name: "runtime requires auto", options: Options{Layout: LayoutFrames, Animation: AnimationCSS, AutoObjective: AutoObjectiveRuntime}},
		{name: "negative FPS", options: Options{Layout: LayoutFrames, Animation: AnimationCSS, MaxFPS: -1}},
		{name: "FPS above limit", options: Options{Layout: LayoutFrames, Animation: AnimationCSS, MaxFPS: maxSVGFPS + 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.options.Validate(); err == nil {
				t.Fatalf("Validate(%#v) succeeded", test.options)
			}
		})
	}
	limit := Options{Layout: LayoutFrames, Animation: AnimationCSS, MaxFPS: maxSVGFPS}
	if err := limit.Validate(); err != nil {
		t.Fatalf("Validate(%#v) error = %v", limit, err)
	}
}

func TestNormalizeFillsDefaultsAndLowercases(t *testing.T) {
	if got := (Options{}).Normalize(); got != DefaultOptions() {
		t.Fatalf("Normalize() of zero options = %#v, want defaults", got)
	}
	got := Options{Layout: "REGIONS", Animation: "SMIL", MaxFPS: 5}.Normalize()
	if got.Layout != LayoutRegions || got.Animation != AnimationSMIL || got.MaxFPS != 5 {
		t.Fatalf("Normalize() = %#v", got)
	}
	if err := DefaultOptions().Validate(); err != nil {
		t.Fatalf("defaults are invalid: %v", err)
	}
}
