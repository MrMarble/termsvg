package svg

import (
	"context"
	"hash/fnv"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

type dynamicRegion struct {
	x            int
	y            int
	width        int
	height       int
	content      timeline[[]ir.Row]
	fallbackRows []ir.Row
}

type regionMeasurement struct {
	regions []dynamicRegion
	bytes   int64
}

type regionMergePair struct {
	i int
	j int
}

type temporalSchedule struct {
	hash  uint64
	times []time.Duration
}

const regionCandidateEvaluationBudget = 512

func buildDynamicRegions(plan *renderPlan, colors *color.Catalog) []dynamicRegion {
	if len(plan.content.points) == 0 {
		return nil
	}
	grids := visualGridsForPlan(plan, colors)

	regions := make([]dynamicRegion, 0)
	for y := range plan.height {
		regions = append(regions, dynamicRegionsForRow(plan, grids, y)...)
	}
	return regions
}

func visualGridsForPlan(plan *renderPlan, colors *color.Catalog) []visualGrid {
	grids := make([]visualGrid, len(plan.content.points))
	for i, point := range plan.content.points {
		grids[i] = buildVisualGrid(plan.width, plan.height, point.state, colors)
	}
	return grids
}

func dynamicRegionsForRow(
	plan *renderPlan,
	grids []visualGrid,
	y int,
) []dynamicRegion {
	for i := range grids {
		if grids[i].rows[y].supported {
			continue
		}
		fallback := rowTimeline(plan, y)
		if len(fallback.points) <= 1 {
			return nil
		}
		rows := make([]ir.Row, len(fallback.points))
		for i, point := range fallback.points {
			rows[i] = point.state
		}
		return []dynamicRegion{{
			y: y, width: max(1, plan.width), height: 1,
			content: rowTimelineRows(fallback), fallbackRows: rows,
		}}
	}

	regions := make([]dynamicRegion, 0)
	atoms := visualAtoms(grids, y, plan.width)
	for index := 0; index < len(atoms); {
		schedule := atomSchedule(plan, grids, y, atoms[index][0], atoms[index][1])
		if len(schedule.times) == 0 {
			index++
			continue
		}
		start, end := atoms[index][0], atoms[index][1]
		index++
		for index < len(atoms) {
			next := atomSchedule(plan, grids, y, atoms[index][0], atoms[index][1])
			if !schedule.equal(next) {
				break
			}
			end = atoms[index][1]
			index++
		}
		candidate := cropRegionFromGrids(plan, grids, start, y, end-start, 1)
		if len(candidate.content.points) > 1 {
			regions = append(regions, candidate)
		}
	}
	return regions
}

func visualAtoms(grids []visualGrid, y, width int) [][2]int {
	joined := make([]bool, width)
	for i := range grids {
		for _, glyph := range grids[i].rows[y].glyphs {
			for col := glyph.startCol + 1; col < glyph.startCol+glyph.width; col++ {
				joined[col] = true
			}
		}
	}
	atoms := make([][2]int, 0, width)
	for start := 0; start < width; {
		end := start + 1
		for end < width && joined[end] {
			end++
		}
		atoms = append(atoms, [2]int{start, end})
		start = end
	}
	return atoms
}

func atomSchedule(plan *renderPlan, grids []visualGrid, y, start, end int) temporalSchedule {
	var schedule temporalSchedule
	for state := 1; state < len(grids); state++ {
		if !visualAtomEqual(&grids[state-1].rows[y], &grids[state].rows[y], start, end) {
			schedule.times = append(schedule.times, plan.content.points[state].time)
		}
	}
	if len(schedule.times) == 0 {
		return schedule
	}
	h := fnv.New64a()
	var value [20]byte
	for _, at := range schedule.times {
		_, _ = h.Write(strconv.AppendInt(value[:0], int64(at), 10))
		_, _ = h.Write([]byte{0})
	}
	schedule.hash = h.Sum64()
	return schedule
}

func visualAtomEqual(a, b *visualRow, start, end int) bool {
	for col := start; col < end; col++ {
		if !visualCellsEqual(a, col, b, col) {
			return false
		}
	}
	return true
}

func (a temporalSchedule) equal(b temporalSchedule) bool {
	return a.hash == b.hash && slices.Equal(a.times, b.times)
}

func (c *canvas) optimizeDynamicRegions(ctx context.Context, regions []dynamicRegion) ([]dynamicRegion, error) {
	return c.optimizeDynamicRegionsWithBudget(ctx, regions, regionCandidateEvaluationBudget)
}

func (c *canvas) optimizeDynamicRegionsWithBudget(
	ctx context.Context,
	regions []dynamicRegion,
	evaluationBudget int,
) ([]dynamicRegion, error) {
	grids := visualGridsForPlan(&c.plan, c.rec.Colors)
	measure := func(candidate []dynamicRegion) (int64, error) {
		probe := *c
		probe.options.Primitives = PrimitiveSnapshots
		content, err := probe.prepareLocalViewports(ctx, probe.regionBands(candidate))
		if err != nil {
			return 0, err
		}
		return costPreparedContent(ctx, &probe, &content)
	}
	if evaluationBudget > 0 && len(mergeableRegionPairs(regions)) > evaluationBudget {
		spatial := spatialDynamicRegions(&c.plan, grids)
		temporalBytes, err := measure(regions)
		if err != nil {
			return nil, err
		}
		spatialBytes, err := measure(spatial)
		if err != nil {
			return nil, err
		}
		if spatialBytes < temporalBytes {
			regions = spatial
		}
	}
	merge := func(candidate []dynamicRegion, i, j int) []dynamicRegion {
		return mergeDynamicRegionsFromGrids(&c.plan, grids, candidate, i, j)
	}
	return optimizeRegionMergesWithBudget(regions, evaluationBudget, measure, merge)
}

//nolint:gocognit // diffs a row's atoms and joins the dynamic runs in one pass
func spatialDynamicRegions(plan *renderPlan, grids []visualGrid) []dynamicRegion {
	regions := make([]dynamicRegion, 0)
	for y := range plan.height {
		supported := true
		for i := range grids {
			supported = supported && grids[i].rows[y].supported
		}
		if !supported {
			regions = append(regions, dynamicRegionsForRow(plan, grids, y)...)
			continue
		}
		atoms := visualAtoms(grids, y, plan.width)
		dynamic := make([]bool, len(atoms))
		for index, atom := range atoms {
			for col := atom[0]; col < atom[1] && !dynamic[index]; col++ {
				for state := 1; state < len(grids); state++ {
					if !visualCellsEqual(&grids[0].rows[y], col, &grids[state].rows[y], col) {
						dynamic[index] = true
						break
					}
				}
			}
		}
		for index := 0; index < len(dynamic); {
			if !dynamic[index] {
				index++
				continue
			}
			start, end := atoms[index][0], atoms[index][1]
			index++
			for index < len(dynamic) && dynamic[index] {
				end = atoms[index][1]
				index++
			}
			candidate := cropRegionFromGrids(plan, grids, start, y, end-start, 1)
			if len(candidate.content.points) > 1 {
				regions = append(regions, candidate)
			}
		}
	}
	return regions
}

func optimizeRegionMerges(
	regions []dynamicRegion,
	measure func([]dynamicRegion) (int64, error),
	merge func([]dynamicRegion, int, int) []dynamicRegion,
) ([]dynamicRegion, error) {
	return optimizeRegionMergesWithBudget(regions, 0, measure, merge)
}

func optimizeRegionMergesWithBudget(
	regions []dynamicRegion,
	evaluationBudget int,
	measure func([]dynamicRegion) (int64, error),
	merge func([]dynamicRegion, int, int) []dynamicRegion,
) ([]dynamicRegion, error) {
	current := slices.Clone(regions)
	cache := make(map[uint64][]regionMeasurement)
	measureCached := func(regions []dynamicRegion) (int64, error) {
		hash := dynamicRegionSetHash(regions)
		for _, cached := range cache[hash] {
			// The hash narrows lookup; DeepEqual makes the ordered content key exact.
			if reflect.DeepEqual(cached.regions, regions) {
				return cached.bytes, nil
			}
		}
		bytes, err := measure(regions)
		if err == nil {
			cache[hash] = append(cache[hash], regionMeasurement{regions: slices.Clone(regions), bytes: bytes})
		}
		return bytes, err
	}
	currentBytes, err := measureCached(current)
	if err != nil {
		return nil, err
	}
	evaluations := 0
	for {
		pairs := mergeableRegionPairs(current)
		if len(pairs) == 0 || evaluationBudget > 0 && evaluations+len(pairs) > evaluationBudget {
			return current, nil
		}
		evaluations += len(pairs)

		bestSavings := int64(0)
		var best []dynamicRegion
		bestBytes := int64(0)
		for _, pair := range pairs {
			candidate := merge(current, pair.i, pair.j)
			candidateBytes, err := measureCached(candidate)
			if err != nil {
				return nil, err
			}
			savings := currentBytes - candidateBytes
			if savings > bestSavings {
				best, bestBytes, bestSavings = candidate, candidateBytes, savings
			}
		}
		if best == nil {
			return current, nil
		}
		current = best
		currentBytes = bestBytes
	}
}

func mergeableRegionPairs(regions []dynamicRegion) []regionMergePair {
	pairs := make([]regionMergePair, 0)
	for i := range regions {
		for j := i + 1; j < len(regions); j++ {
			if dynamicRegionsMergeable(&regions[i], &regions[j]) &&
				!mergedRegionIntersectsOther(regions, i, j) {
				pairs = append(pairs, regionMergePair{i: i, j: j})
			}
		}
	}
	return pairs
}

func dynamicRegionSetHash(regions []dynamicRegion) uint64 {
	h := fnv.New64a()
	var value [20]byte
	addInt := func(n int64) {
		_, _ = h.Write(strconv.AppendInt(value[:0], n, 10))
		_, _ = h.Write([]byte{0})
	}
	addUint := func(n uint64) {
		_, _ = h.Write(strconv.AppendUint(value[:0], n, 10))
		_, _ = h.Write([]byte{0})
	}
	addRows := func(rows []ir.Row) {
		addInt(int64(len(rows)))
		for _, row := range rows {
			addUint(semanticRowHash(row))
		}
	}
	addInt(int64(len(regions)))
	for _, region := range regions {
		addInt(int64(region.x))
		addInt(int64(region.y))
		addInt(int64(region.width))
		addInt(int64(region.height))
		addInt(int64(region.content.duration))
		addInt(int64(len(region.content.points)))
		for _, point := range region.content.points {
			addInt(int64(point.time))
			addRows(point.state)
		}
		addRows(region.fallbackRows)
	}
	return h.Sum64()
}

func mergedRegionIntersectsOther(regions []dynamicRegion, i, j int) bool {
	a, b := regions[i], regions[j]
	merged := dynamicRegion{
		x: min(a.x, b.x), y: min(a.y, b.y),
	}
	merged.width = max(a.x+a.width, b.x+b.width) - merged.x
	merged.height = max(a.y+a.height, b.y+b.height) - merged.y
	for index := range regions {
		if index != i && index != j && dynamicRegionRectsIntersect(&merged, &regions[index]) {
			return true
		}
	}
	return false
}

func dynamicRegionRectsIntersect(a, b *dynamicRegion) bool {
	return a.x < b.x+b.width && b.x < a.x+a.width &&
		a.y < b.y+b.height && b.y < a.y+a.height
}

func dynamicRegionsMergeable(a, b *dynamicRegion) bool {
	if len(a.fallbackRows) > 0 || len(b.fallbackRows) > 0 {
		return false
	}
	vertical := (a.y+a.height == b.y || b.y+b.height == a.y) &&
		a.x <= b.x+b.width && b.x <= a.x+a.width
	horizontal := (a.x+a.width == b.x || b.x+b.width == a.x) &&
		a.y <= b.y+b.height && b.y <= a.y+a.height
	return vertical || horizontal
}

func mergeDynamicRegions(
	plan *renderPlan,
	regions []dynamicRegion,
	i, j int,
	colors *color.Catalog,
) []dynamicRegion {
	return mergeDynamicRegionsFromGrids(plan, visualGridsForPlan(plan, colors), regions, i, j)
}

func mergeDynamicRegionsFromGrids(
	plan *renderPlan,
	grids []visualGrid,
	regions []dynamicRegion,
	i, j int,
) []dynamicRegion {
	a, b := regions[i], regions[j]
	x, y := min(a.x, b.x), min(a.y, b.y)
	endX, endY := max(a.x+a.width, b.x+b.width), max(a.y+a.height, b.y+b.height)
	merged := cropRegionFromGrids(plan, grids, x, y, endX-x, endY-y)
	out := make([]dynamicRegion, 0, len(regions)-1)
	for index, region := range regions {
		if index != i && index != j {
			out = append(out, region)
		}
	}
	out = append(out, merged)
	sort.Slice(out, func(i, j int) bool {
		a, b := &out[i], &out[j]
		if a.y != b.y {
			return a.y < b.y
		}
		if a.x != b.x {
			return a.x < b.x
		}
		if a.height != b.height {
			return a.height < b.height
		}
		return a.width < b.width
	})
	return out
}

func rowTimeline(plan *renderPlan, y int) timeline[ir.Row] {
	points := make([]timelinePoint[ir.Row], len(plan.content.points))
	for i, point := range plan.content.points {
		points[i] = timelinePoint[ir.Row]{time: point.time, state: rowAt(point.state, y)}
	}
	return normalizeTimeline(plan.duration, points, rowEqual)
}

func rowTimelineRows(content timeline[ir.Row]) timeline[[]ir.Row] {
	points := make([]timelinePoint[[]ir.Row], len(content.points))
	for i, point := range content.points {
		rows := []ir.Row(nil)
		if len(point.state.Runs) > 0 {
			row := point.state
			row.Y = 0
			row.Runs = slices.Clone(row.Runs)
			rows = []ir.Row{row}
		}
		points[i] = timelinePoint[[]ir.Row]{time: point.time, state: rows}
	}
	return timeline[[]ir.Row]{duration: content.duration, points: points}
}

func cropRegionFromGrids(
	plan *renderPlan,
	grids []visualGrid,
	x, y, width, height int,
) dynamicRegion {
	points := make([]timelinePoint[[]ir.Row], len(plan.content.points))
	for i := range plan.content.points {
		grid := &grids[i]
		rows := make([]ir.Row, 0, height)
		for terminalY := y; terminalY < y+height; terminalY++ {
			if row := cropVisualRow(&grid.rows[terminalY], x, x+width, terminalY-y); len(row.Runs) > 0 {
				rows = append(rows, row)
			}
		}
		points[i] = timelinePoint[[]ir.Row]{time: plan.content.points[i].time, state: rows}
	}
	return dynamicRegion{
		x: x, y: y, width: width, height: height,
		content: normalizeTimeline(plan.duration, points, rowsEqual),
	}
}

func cropVisualRow(row *visualRow, start, end, y int) ir.Row {
	out := ir.Row{Y: y}
	for _, glyph := range row.glyphs {
		if glyph.startCol < start || glyph.startCol+glyph.width > end {
			continue
		}
		if glyph.text == " " && !glyph.attrs.Underline {
			cell := row.cells[glyph.startCol]
			if !cell.visible {
				continue
			}
		}
		localStart := glyph.startCol - start
		if n := len(out.Runs); n > 0 && out.Runs[n-1].Attrs == glyph.attrs && out.Runs[n-1].EndCol == localStart {
			out.Runs[n-1].Text += glyph.text
			out.Runs[n-1].EndCol += glyph.width
			continue
		}
		out.Runs = append(out.Runs, ir.TextRun{
			Text: glyph.text, StartCol: localStart, EndCol: localStart + glyph.width, Attrs: glyph.attrs,
		})
	}
	return out
}
