package svg

import "github.com/mrmarble/termsvg/internal/color"

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
