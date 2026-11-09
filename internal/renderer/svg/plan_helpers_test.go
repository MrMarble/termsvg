package svg

import (
	"context"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/renderer"
)

func buildRenderPlan(rec *ir.Recording, showCursor bool) semanticPlan {
	options := DefaultOptions()
	return buildRenderPlanWithOptions(rec, showCursor, &options)
}

func buildRenderPlanWithOptions(rec *ir.Recording, showCursor bool, options *Options) semanticPlan {
	plan, err := buildSemanticPlan(context.Background(), rec, showCursor, options.MaxFPS, 1)
	if err != nil {
		panic(err)
	}
	return plan
}

// layeredPlan builds the semantic plan and selects its interval layers.
func layeredPlan(
	ctx context.Context,
	rec *ir.Recording,
	config *renderer.Config,
	options *Options,
) (semanticPlan, error) {
	plan, err := buildSemanticPlan(ctx, rec, config.ShowCursor, options.MaxFPS, config.LoopCount)
	if err != nil {
		return semanticPlan{}, err
	}
	return New(config, WithOptions(*options)).selectIntervalLayers(ctx, rec, &plan)
}
