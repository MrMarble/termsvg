package svg

import (
	"context"
	"fmt"
	"io"

	"github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
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

func (c *canvas) renderRegionRepresentation(ctx context.Context, w io.Writer, content *preparedContent) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	probe := *c
	probe.w = w
	probe.metrics = &CandidateMetrics{}
	probe.plan.cursor = timeline[ir.Cursor]{}
	probe.plan.cursorEverVisible = false
	fmt.Fprint(w, `<svg><defs>`)
	probe.writeRowDefs(content.rowDefs)
	probe.writeStateDefs(content)
	fmt.Fprint(w, `</defs>`)
	probe.writeStyles(content)
	probe.writeContent(content)
	fmt.Fprint(w, `</svg>`)
	return contextErr(ctx)
}
