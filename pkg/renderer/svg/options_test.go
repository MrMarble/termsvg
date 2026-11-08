//nolint:lll // Compact option literals make defaults easy to compare.
package svg

import (
	"testing"

	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestDefaultOptionsPreserveCompatibilityPath(t *testing.T) {
	got := DefaultOptions()
	want := Options{Layout: LayoutFrames, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate, AutoObjective: AutoObjectiveSize, Style: StyleLegacy, Primitives: PrimitiveSnapshots}
	if got != want {
		t.Fatalf("DefaultOptions() = %#v, want %#v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("default options are invalid: %v", err)
	}
}

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

func TestRegionsLayoutValidates(t *testing.T) {
	options := DefaultOptions()
	options.Layout = LayoutRegions
	if err := options.Validate(); err != nil {
		t.Fatalf("regions layout failed validation: %v", err)
	}
}

func TestScrollLayoutValidatesWithoutChangingDefault(t *testing.T) {
	if DefaultOptions().Layout != LayoutFrames {
		t.Fatalf("default layout = %q, want %q", DefaultOptions().Layout, LayoutFrames)
	}
	options := DefaultOptions()
	options.Layout = LayoutScroll
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

func TestNewAcceptsFunctionalOptionsWithoutChangingExistingCallers(t *testing.T) {
	config := renderer.DefaultConfig()
	if got := New(config); got.options != DefaultOptions() {
		t.Fatalf("New(config) options = %#v", got.options)
	}
	got := New(config, WithLayout(LayoutAuto), WithAnimation(AnimationSMIL), WithMaxFPS(30), WithAutoObjective(AutoObjectiveRuntime), WithStyleMode(StyleAuto))
	want := Options{
		Layout: LayoutAuto, Animation: AnimationSMIL, FrameSwitch: FrameSwitchTranslate, MaxFPS: 30,
		AutoObjective: AutoObjectiveRuntime, Style: StyleAuto, Primitives: PrimitiveSnapshots,
	}
	if got.options != want {
		t.Fatalf("functional options = %#v, want %#v", got.options, want)
	}
}
