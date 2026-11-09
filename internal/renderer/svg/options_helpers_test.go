package svg

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
