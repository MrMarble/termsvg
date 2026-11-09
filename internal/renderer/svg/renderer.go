// Package svg provides an SVG renderer for terminal recordings.
// It generates animated SVGs using CSS keyframes or discrete SMIL timelines.
package svg

import (
	"context"
	"io"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/layout"
	"github.com/mrmarble/termsvg/internal/renderer"
)

// Renderer implements the renderer.Renderer interface for SVG output.
type Renderer struct {
	config  renderer.Config
	options Options
}

// Layout constants.
const (
	RowHeight  = layout.RowHeight
	ColWidth   = layout.ColWidth
	Padding    = layout.Padding
	HeaderSize = layout.HeaderSize

	windowCornerRadius = 5
)

func (r *Renderer) buildSemanticPlan(ctx context.Context, rec *ir.Recording) (semanticPlan, error) {
	return buildSemanticPlan(ctx, rec, r.config.ShowCursor, r.options.MaxFPS, r.config.LoopCount)
}

func New(config *renderer.Config, opts ...Option) *Renderer {
	options := DefaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return &Renderer{config: *config, options: options.Normalize()}
}

func (r *Renderer) Format() string {
	return "svg"
}

func (r *Renderer) FileExtension() string {
	return ".svg"
}

// Render generates an animated SVG from the recording. With config.Minify the
// output is minified as it is written.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	candidate, err := r.prepareBestCandidate(ctx, rec)
	if err != nil {
		return err
	}
	return writeFinalSVG(w, r.config.Minify, func(w io.Writer) error {
		return r.serializeCandidate(ctx, rec, w, candidate)
	})
}
