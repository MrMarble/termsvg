package svg

import "fmt"

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
	// LayoutAuto measures the concrete frame and band serializations and emits
	// the smaller one. It is opt-in because it performs an extra render pass.
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

// WithLayout selects the SVG content layout.
func WithLayout(layout LayoutMode) Option {
	return func(options *Options) { options.Layout = layout }
}

// WithAnimation selects the SVG animation backend.
func WithAnimation(animation AnimationMode) Option {
	return func(options *Options) { options.Animation = animation }
}

// WithFrameSwitch selects translated strips or experimental animated hrefs.
func WithFrameSwitch(frameSwitch FrameSwitchMode) Option {
	return func(options *Options) { options.FrameSwitch = frameSwitch }
}

// WithMaxFPS enables opt-in lossy timeline sampling. Zero keeps all states.
func WithMaxFPS(maxFPS int) Option {
	return func(options *Options) { options.MaxFPS = maxFPS }
}

// WithAutoObjective selects size or structural runtime proxy comparison for auto layout.
func WithAutoObjective(objective AutoObjective) Option {
	return func(options *Options) { options.AutoObjective = objective }
}

// WithStyleMode selects the compatibility or profitability-driven paint encoding.
func WithStyleMode(style StyleMode) Option {
	return func(options *Options) { options.Style = style }
}

// WithPrimitiveMode selects snapshot or retained rectangle emission.
func WithPrimitiveMode(primitives PrimitiveMode) Option {
	return func(options *Options) { options.Primitives = primitives }
}

func (o *Options) normalized() Options {
	n := *o
	if n.Layout == "" {
		n.Layout = LayoutFrames
	}
	if n.Animation == "" {
		n.Animation = AnimationCSS
	}
	if n.FrameSwitch == "" {
		n.FrameSwitch = FrameSwitchTranslate
	}
	if n.AutoObjective == "" {
		n.AutoObjective = AutoObjectiveSize
	}
	if n.Style == "" {
		n.Style = StyleLegacy
	}
	if n.Primitives == "" {
		n.Primitives = PrimitiveSnapshots
	}
	return n
}

// Validate checks SVG-specific options.
func (o *Options) Validate() error {
	normalized := o.normalized()
	o = &normalized
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
