package svg

import (
	"context"
	"io"

	"github.com/mrmarble/termsvg/internal/color"
)

func optimizeRegionMerges(
	regions []dynamicRegion,
	measure func([]dynamicRegion) (int64, error),
	merge func([]dynamicRegion, int, int) []dynamicRegion,
) ([]dynamicRegion, error) {
	return optimizeRegionMergesWithBudget(regions, 0, measure, merge)
}

func mergeDynamicRegions(
	plan *semanticPlan,
	regions []dynamicRegion,
	i, j int,
	colors *color.Catalog,
) []dynamicRegion {
	return mergeDynamicRegionsFromGrids(plan, visualGridsForPlan(plan, colors), regions, i, j)
}

func (c *canvas) serializedRegionBytes(ctx context.Context, regions []dynamicRegion) (int64, error) {
	content, err := c.prepareLocalViewports(ctx, c.regionBands(regions))
	if err != nil {
		return 0, err
	}
	counter := &countingWriter{}
	if err := writeFinalSVG(counter, c.config.Minify, func(w io.Writer) error {
		return c.renderRegionRepresentation(ctx, w, &content)
	}); err != nil {
		return 0, err
	}
	return counter.size(), nil
}
