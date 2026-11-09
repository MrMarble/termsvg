package svg

import (
	"fmt"
	"strings"
)

// LayoutMode selects how changing terminal content is arranged in the SVG.
type LayoutMode string

// AnimationMode selects the SVG animation mechanism.
type AnimationMode string

// FrameSwitchMode selects how discrete content states are activated.
type FrameSwitchMode string

// AutoObjective selects how auto layout candidates are compared.
type AutoObjective string

// StyleMode selects the SVG paint encoding.
type StyleMode string

// PrimitiveMode selects snapshot or retained SVG primitive emission.
type PrimitiveMode string

// Options contains SVG-specific renderer settings.
type Options struct {
	Layout        LayoutMode
	Animation     AnimationMode
	FrameSwitch   FrameSwitchMode
	AutoObjective AutoObjective
	Style         StyleMode
	Primitives    PrimitiveMode
	// MaxFPS limits SVG timeline samples. Zero preserves every source state.
	MaxFPS int
	// withoutBlockSharing keeps recurring row groups inline. Tests use it to
	// measure what block sharing saves.
	withoutBlockSharing bool
}

// Option updates SVG renderer options.
type Option func(*Options)

const (
	// LayoutFrames retains the compatibility layout: complete dynamic states are
	// placed in one horizontal strip.
	LayoutFrames LayoutMode = "frames"
	// LayoutBands builds independently animated horizontal strips for adjacent
	// rows that share a change schedule.
	LayoutBands LayoutMode = "bands"
	// LayoutRegions builds independently animated two-dimensional viewports.
	LayoutRegions LayoutMode = "regions"
	// LayoutScroll converts strictly proven upward-scrolling bands to clipped
	// vertical tapes and leaves every other band on the lossless snapshot path.
	LayoutScroll LayoutMode = "scroll"
	// LayoutAuto measures the concrete frames, bands and regions serializations
	// and emits the one AutoObjective prefers. It is opt-in because it renders
	// every candidate, and regions can take a while on large recordings.
	LayoutAuto LayoutMode = "auto"

	// AnimationCSS emits CSS keyframes.
	AnimationCSS AnimationMode = "css"
	// AnimationSMIL emits discrete SVG animation elements.
	AnimationSMIL AnimationMode = "smil"

	// FrameSwitchTranslate places states in a translated strip.
	FrameSwitchTranslate FrameSwitchMode = "translate"
	// FrameSwitchHref animates one use element between state definitions.
	FrameSwitchHref FrameSwitchMode = "href"

	AutoObjectiveSize    AutoObjective = "size"
	AutoObjectiveRuntime AutoObjective = "runtime"

	StyleLegacy StyleMode = "legacy"
	StyleAuto   StyleMode = "auto"

	PrimitiveSnapshots  PrimitiveMode = "snapshots"
	PrimitiveRectTracks PrimitiveMode = "rect-tracks"
)

// maxSVGFPS is the highest accepted MaxFPS. Finer sampling is invisible on
// any display, and zero already keeps every state.
const maxSVGFPS = 1000

func (o *Options) usesLocalViewports() bool {
	return o.Layout == LayoutBands || o.Layout == LayoutRegions || o.Layout == LayoutScroll
}

// DefaultOptions returns the compatibility-preserving SVG defaults.
func DefaultOptions() Options {
	return Options{
		Layout:        LayoutFrames,
		Animation:     AnimationCSS,
		FrameSwitch:   FrameSwitchTranslate,
		AutoObjective: AutoObjectiveSize,
		Style:         StyleLegacy,
		Primitives:    PrimitiveSnapshots,
	}
}

// WithOptions replaces the renderer's options with o.
func WithOptions(o Options) Option {
	return func(options *Options) { *options = o }
}

// Normalize returns o with every unset field replaced by its default and
// every enum value lower-cased.
func (o Options) Normalize() Options {
	d := DefaultOptions()
	fill := func(value, def string) string {
		if value == "" {
			return def
		}
		return strings.ToLower(value)
	}
	o.Layout = LayoutMode(fill(string(o.Layout), string(d.Layout)))
	o.Animation = AnimationMode(fill(string(o.Animation), string(d.Animation)))
	o.FrameSwitch = FrameSwitchMode(fill(string(o.FrameSwitch), string(d.FrameSwitch)))
	o.AutoObjective = AutoObjective(fill(string(o.AutoObjective), string(d.AutoObjective)))
	o.Style = StyleMode(fill(string(o.Style), string(d.Style)))
	o.Primitives = PrimitiveMode(fill(string(o.Primitives), string(d.Primitives)))
	return o
}

func (o Options) Validate() error {
	o = o.Normalize()
	switch o.Layout {
	case LayoutFrames, LayoutBands, LayoutRegions, LayoutScroll, LayoutAuto:
	default:
		return fmt.Errorf("unsupported SVG layout %q", o.Layout)
	}
	switch o.Animation {
	case AnimationCSS, AnimationSMIL:
	default:
		return fmt.Errorf("unsupported SVG animation mode %q", o.Animation)
	}
	switch o.FrameSwitch {
	case FrameSwitchTranslate, FrameSwitchHref:
	default:
		return fmt.Errorf("unsupported SVG frame switch mode %q", o.FrameSwitch)
	}
	switch o.AutoObjective {
	case AutoObjectiveSize, AutoObjectiveRuntime:
	default:
		return fmt.Errorf("unsupported SVG auto objective %q", o.AutoObjective)
	}
	switch o.Style {
	case StyleLegacy, StyleAuto:
	default:
		return fmt.Errorf("unsupported SVG style mode %q", o.Style)
	}
	switch o.Primitives {
	case PrimitiveSnapshots, PrimitiveRectTracks:
	default:
		return fmt.Errorf("unsupported SVG primitive mode %q", o.Primitives)
	}
	if o.Primitives == PrimitiveRectTracks && o.Animation != AnimationSMIL {
		return fmt.Errorf("SVG rectangle tracks require SMIL animation")
	}
	if o.Primitives == PrimitiveRectTracks && o.Layout != LayoutRegions && o.Layout != LayoutScroll {
		return fmt.Errorf("SVG rectangle tracks require regions or scroll layout")
	}
	if o.AutoObjective != AutoObjectiveSize && o.Layout != LayoutAuto {
		return fmt.Errorf("SVG auto objective %q requires auto layout", o.AutoObjective)
	}
	if o.FrameSwitch == FrameSwitchHref && o.Animation != AnimationSMIL {
		return fmt.Errorf("SVG href frame switching requires SMIL animation")
	}
	if o.MaxFPS < 0 {
		return fmt.Errorf("max SVG FPS must not be negative: %d", o.MaxFPS)
	}
	if o.MaxFPS > maxSVGFPS {
		return fmt.Errorf("max SVG FPS must not exceed %d: %d", maxSVGFPS, o.MaxFPS)
	}
	return nil
}
