// Package svg provides an SVG renderer for terminal recordings.
// It generates animated SVGs using CSS keyframes or discrete SMIL timelines.
package svg

import (
	"context"
	"fmt"
	"io"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/layout"
	"github.com/mrmarble/termsvg/internal/renderer"
)

// Renderer implements the renderer.Renderer interface for SVG output.
type Renderer struct {
	config               renderer.Config
	options              Options
	onSemanticPlanBuild  func()
	onCandidateSerialize func()
}

// Layout constants, shared with the raster renderers.
const (
	RowHeight  = layout.RowHeight
	ColWidth   = layout.ColWidth
	Padding    = layout.Padding
	HeaderSize = layout.HeaderSize

	windowCornerRadius = 5
)

func (r *Renderer) buildSemanticPlan(ctx context.Context, rec *ir.Recording) (semanticPlan, error) {
	if r.onSemanticPlanBuild != nil {
		r.onSemanticPlanBuild()
	}
	return buildSemanticPlan(ctx, rec, r.config.ShowCursor, r.options.MaxFPS, r.config.LoopCount)
}

// New creates a new SVG renderer with the given configuration.
func New(config *renderer.Config, opts ...Option) *Renderer {
	options := DefaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return &Renderer{config: *config, options: options.normalized()}
}

// Format returns the output format name.
func (r *Renderer) Format() string {
	return "svg"
}

// FileExtension returns the file extension for SVG files.
func (r *Renderer) FileExtension() string {
	return ".svg"
}

// Render generates an animated SVG from the recording.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	if len(rec.Frames) == 0 {
		return fmt.Errorf("recording has no frames")
	}
	if err := r.options.Validate(); err != nil {
		return err
	}
	candidate, err := r.prepareBestCandidate(ctx, rec)
	if err != nil {
		return err
	}
	return r.serializeCandidate(ctx, rec, w, candidate)
}
