// Package svg provides an SVG renderer for terminal recordings.
// It generates animated SVGs using CSS keyframes or discrete SMIL timelines.
package svg

import (
	"context"
	"fmt"
	"io"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/renderer"
)

// Renderer implements the renderer.Renderer interface for SVG output.
type Renderer struct {
	config               renderer.Config
	options              Options
	onSemanticPlanBuild  func()
	onCandidateSerialize func()
}

// Layout constants for SVG rendering
const (
	RowHeight  = 25 // pixels per row
	ColWidth   = 12 // pixels per column
	Padding    = 20 // padding around content
	HeaderSize = 2  // multiplier for header area (window buttons)

	// windowCornerRadius is the radius for rounded window corners.
	windowCornerRadius = 5

	// windowButtonSpacing is the horizontal spacing between window buttons.
	windowButtonSpacing = 20

	// windowButtonRadius is the radius of window control buttons.
	windowButtonRadius = 6
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
