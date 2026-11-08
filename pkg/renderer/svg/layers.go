package svg

import (
	"context"
	"fmt"
	"io"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// intervalLayer holds cells that keep one visual value over a contiguous run
// of content states. The layer is drawn once and its visibility is animated,
// so the cells disappear from every state in the interval. Whole-recording
// static cells are hoisted earlier by hoistStaticCells; this pass covers the
// common case of a blank first frame, a cleared last frame, or labels that
// stay for part of a recording.
type intervalLayer struct {
	rows []ir.Row
	from time.Duration // first instant the layer is visible
	to   time.Duration // instant the layer hides again, unless open
	open bool          // the interval includes the final state, so the layer never hides
}

type cellInterval struct {
	y, col int
}

// layerCandidate is a set of cells that share one constant interval.
type layerCandidate struct {
	from, to int // inclusive content state indices
	cells    []cellInterval
	estimate int
}

// layerCandidateSet holds the decoded cell grid the candidates refer to.
type layerCandidateSet struct {
	width      int
	decoded    [][][]terminalCell        // [y][state][col]; nil rows could not be decoded
	runs       map[cellInterval][][2]int // maximal constant runs of every visible cell
	candidates []*layerCandidate
}

// layerRanker estimates candidate savings against the rows as they are
// serialized today, including whole-row references and shared segments.
type layerRanker struct {
	c      *canvas
	set    *layerCandidateSet
	frames [][]*renderedRow
	sb     strings.Builder
}

const (
	// layerAnimationBytes approximates one visibility animation with its
	// group wrapper when ordering candidates.
	layerAnimationBytes = 160
	// maxMeasuredLayerCandidates bounds the exact measurements per render.
	maxMeasuredLayerCandidates = 64
	// maxLayerRejections stops the exact trials after this many consecutive
	// candidates failed to shrink the plan.
	maxLayerRejections = 16
	// rowReferenceBytes is the serialized size of a whole-row <use> reference.
	rowReferenceBytes = len(`<use href="#ab"/>`)
)

// intervalLayerCandidates finds every distinct interval over which some cell
// is constant for at least two states but not the whole recording. A
// candidate takes every cell that is constant over its interval, including
// cells whose own constant run is longer, so labels that the moving parts of
// a screen interrupt at different times still share one layer.
func (p *renderPlan) intervalLayerCandidates(width, height int, colors *color.Catalog) layerCandidateSet {
	n := len(p.content.points)
	set := layerCandidateSet{
		width: width, decoded: make([][][]terminalCell, height), runs: make(map[cellInterval][][2]int),
	}
	if width <= 0 || n < 3 {
		return set
	}
	byInterval := make(map[[2]int]*layerCandidate)
	cells := make([]cellInterval, 0)
	for y := range height {
		states, ok := p.decodeRowStates(y, width)
		if !ok {
			continue
		}
		set.decoded[y] = states
		for col := range width {
			cell := cellInterval{y: y, col: col}
			constantCellRuns(states, col, colors, func(from, to int) {
				if len(set.runs[cell]) == 0 {
					cells = append(cells, cell)
				}
				set.runs[cell] = append(set.runs[cell], [2]int{from, to})
				key := [2]int{from, to}
				if _, ok := byInterval[key]; !ok {
					candidate := &layerCandidate{from: from, to: to}
					byInterval[key] = candidate
					set.candidates = append(set.candidates, candidate)
				}
			})
		}
	}
	for _, candidate := range set.candidates {
		for _, cell := range cells {
			if set.constantOver(cell, candidate.from, candidate.to) {
				candidate.cells = append(candidate.cells, cell)
			}
		}
	}
	slices.SortFunc(set.candidates, func(a, b *layerCandidate) int {
		if a.from != b.from {
			return a.from - b.from
		}
		return a.to - b.to
	})
	return set
}

// constantOver reports whether the cell keeps one visible value over the
// whole state interval.
func (set *layerCandidateSet) constantOver(cell cellInterval, from, to int) bool {
	for _, run := range set.runs[cell] {
		if run[0] <= from && to <= run[1] {
			return true
		}
	}
	return false
}

// uncovered returns the candidate cells that no accepted layer on an
// intersecting interval has taken yet.
func (candidate *layerCandidate) uncovered(covered map[cellInterval][][2]int) []cellInterval {
	cells := make([]cellInterval, 0, len(candidate.cells))
	for _, cell := range candidate.cells {
		taken := false
		for _, interval := range covered[cell] {
			if interval[0] <= candidate.to && candidate.from <= interval[1] {
				taken = true
				break
			}
		}
		if !taken {
			cells = append(cells, cell)
		}
	}
	return cells
}

// decodeRowStates decodes row y of every content state into cells.
func (p *renderPlan) decodeRowStates(y, width int) ([][]terminalCell, bool) {
	states := make([][]terminalCell, len(p.content.points))
	for i, point := range p.content.points {
		var ok bool
		if states[i], ok = decodeRowCells(rowAt(point.state, y), width); !ok {
			return nil, false
		}
	}
	return states, true
}

// constantCellRuns reports every run of at least two consecutive states in
// which the cell keeps one visible value, except a run over every state.
func constantCellRuns(states [][]terminalCell, col int, colors *color.Catalog, visit func(from, to int)) {
	n := len(states)
	start := 0
	for i := 1; i <= n; i++ {
		if i < n && cellVisualEqual(states[start][col], states[i][col], colors) {
			continue
		}
		if i-start >= 2 && cellVisible(states[start][col], colors) && (start > 0 || i < n) {
			visit(start, i-1)
		}
		start = i
	}
}

func (candidate *layerCandidate) masks(width int) map[int][]bool {
	byRow := make(map[int][]bool)
	for _, cell := range candidate.cells {
		mask, ok := byRow[cell.y]
		if !ok {
			mask = make([]bool, width)
			byRow[cell.y] = mask
		}
		mask[cell.col] = true
	}
	return byRow
}

func sortedRowKeys(m map[int][]bool) []int {
	keys := make([]int, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// withIntervalLayers returns a copy of the plan in which the selected
// candidates are hoisted into layers and removed from the states they cover.
func (p *renderPlan) withIntervalLayers(
	set *layerCandidateSet,
	selected []*layerCandidate,
	colors *color.Catalog,
) renderPlan {
	plan := *p
	plan.layers = nil
	if len(selected) == 0 {
		return plan
	}
	n := len(p.content.points)
	removed := make(map[[2]int][]bool) // (y, state) -> removed columns
	for _, candidate := range selected {
		layer := intervalLayer{from: p.content.points[candidate.from].time, to: p.duration, open: true}
		if candidate.to+1 < n {
			// A following state may start exactly at the duration; the layer
			// still hides there so a frozen final frame matches that state.
			layer.to, layer.open = p.content.points[candidate.to+1].time, false
		}
		byRow := candidate.masks(set.width)
		for _, y := range sortedRowKeys(byRow) {
			layer.rows = append(layer.rows, cellsToRow(y, set.decoded[y][candidate.from], byRow[y], colors))
			for state := candidate.from; state <= candidate.to; state++ {
				key := [2]int{y, state}
				if removed[key] == nil {
					removed[key] = make([]bool, set.width)
				}
				for col, hoisted := range byRow[y] {
					removed[key][col] = removed[key][col] || hoisted
				}
			}
		}
		plan.layers = append(plan.layers, layer)
	}
	points := slices.Clone(p.content.points)
	for key, mask := range removed {
		y, state := key[0], key[1]
		keep := make([]bool, set.width)
		for col := range set.width {
			keep[col] = !mask[col] && cellVisible(set.decoded[y][state][col], colors)
		}
		points[state].state = replaceRow(points[state].state, cellsToRow(y, set.decoded[y][state], keep, colors))
	}
	plan.content = normalizeTimeline(p.duration, points, rowsEqual)
	return plan
}

// selectIntervalLayers chooses layers greedily: candidates are ordered by an
// estimate and each is kept only when the exact frames serialization of the
// plan shrinks. The caller still compares the layered and plain plans on the
// configured layout.
func (r *Renderer) selectIntervalLayers(ctx context.Context, rec *ir.Recording, base *renderPlan) (renderPlan, error) {
	set := base.intervalLayerCandidates(rec.Width, rec.Height, rec.Colors)
	if len(set.candidates) == 0 {
		return *base, nil
	}
	// Measure on the configured layout when it is cheap to prepare; the region
	// optimizer and the auto selection are too expensive to run per candidate,
	// so they are approximated by the frames strip. The paint style is fixed to
	// legacy because the auto style search multiplies the cost without changing
	// which layers pay off.
	probe := r.options
	if probe.Layout == LayoutRegions || probe.Layout == LayoutAuto {
		probe.Layout = LayoutFrames
	}
	probe.Style = StyleLegacy
	probe.Primitives = PrimitiveSnapshots
	probe.AutoObjective = AutoObjectiveSize
	c := &canvas{rec: rec, plan: *base, config: r.config, options: probe, classNames: rec.Colors.GenerateClassNames()}
	c.style = stylePlan{scheme: styleLegacy}
	ranked := c.rankLayerCandidates(&set)
	if len(ranked) > maxMeasuredLayerCandidates {
		if r.config.Debug {
			log.Printf("[SVG] measuring %d of %d interval layer candidates", maxMeasuredLayerCandidates, len(ranked))
		}
		ranked = ranked[:maxMeasuredLayerCandidates]
	}

	return r.trialIntervalLayers(ctx, rec, base, &set, ranked, &probe)
}

// trialIntervalLayers accepts ranked candidates one at a time, keeping each
// only when the exact serialized size of the probe layout shrinks.
func (r *Renderer) trialIntervalLayers(
	ctx context.Context,
	rec *ir.Recording,
	base *renderPlan,
	set *layerCandidateSet,
	ranked []*layerCandidate,
	probe *Options,
) (renderPlan, error) {
	measure := func(plan *renderPlan) (int64, error) {
		candidate, err := prepareCandidate(ctx, rec, plan, &r.config, probe)
		if err != nil {
			return 0, err
		}
		return candidate.cost.finalBytes, nil
	}
	selected := make([]*layerCandidate, 0, len(ranked))
	covered := make(map[cellInterval][][2]int)
	best := *base
	bestBytes, err := measure(&best)
	if err != nil {
		return renderPlan{}, err
	}
	rejections := 0
	for _, ranked := range ranked {
		if err := contextErr(ctx); err != nil {
			return renderPlan{}, err
		}
		if rejections >= maxLayerRejections {
			break
		}
		// Cells an accepted layer already hoists on an overlapping interval
		// belong to that layer; the candidate keeps the rest.
		candidate := *ranked
		if candidate.cells = ranked.uncovered(covered); len(candidate.cells) == 0 {
			continue
		}
		trial := base.withIntervalLayers(set, append(slices.Clone(selected), &candidate), rec.Colors)
		bytes, err := measure(&trial)
		if err != nil {
			return renderPlan{}, err
		}
		if r.config.Debug {
			log.Printf("[SVG] interval layer states=%d..%d cells=%d estimate=%d measured=%d accepted=%t",
				candidate.from, candidate.to, len(candidate.cells), candidate.estimate, bestBytes-bytes, bytes < bestBytes)
		}
		if bytes >= bestBytes {
			rejections++
			continue
		}
		rejections = 0
		selected = append(selected, &candidate)
		best, bestBytes = trial, bytes
		for _, cell := range candidate.cells {
			covered[cell] = append(covered[cell], [2]int{candidate.from, candidate.to})
		}
	}
	return best, nil
}

// rankLayerCandidates orders candidates by estimated savings on the frames
// serialization and drops those that cannot pay for their animation.
func (c *canvas) rankLayerCandidates(set *layerCandidateSet) []*layerCandidate {
	states := make([][]ir.Row, len(c.plan.content.points))
	for i, point := range c.plan.content.points {
		states[i] = point.state
	}
	ranker := layerRanker{c: c, set: set}
	ranker.frames, _ = c.collectRows(states)
	ranked := make([]*layerCandidate, 0, len(set.candidates))
	for _, candidate := range set.candidates {
		candidate.estimate = -layerAnimationBytes
		kept := make([]cellInterval, 0, len(candidate.cells))
		masks := candidate.masks(set.width)
		for _, y := range sortedRowKeys(masks) {
			for _, group := range cellGroups(masks[y]) {
				savings := ranker.groupSavings(candidate, y, group)
				if savings <= 0 {
					continue
				}
				candidate.estimate += savings
				for col, hoisted := range group {
					if hoisted {
						kept = append(kept, cellInterval{y: y, col: col})
					}
				}
			}
		}
		candidate.cells = kept
		if candidate.estimate > 0 && len(kept) > 0 {
			ranked = append(ranked, candidate)
		}
	}
	slices.SortStableFunc(ranked, func(a, b *layerCandidate) int { return b.estimate - a.estimate })
	return ranked
}

func (r *layerRanker) rowCost(state, y int) int {
	for _, rendered := range r.frames[state] {
		if len(rendered.rows) > 0 {
			// Rows drawn through a shared block are estimated like whole-row
			// references: their markup lives in a definition.
			if slices.ContainsFunc(rendered.rows, func(row ir.Row) bool { return row.Y == y }) {
				return rowReferenceBytes
			}
			continue
		}
		if rendered.row.Y != y {
			continue
		}
		if rendered.id != "" {
			return rowReferenceBytes
		}
		return finalSVGBytes(rendered.svg, r.c.config.Minify)
	}
	return 0
}

func (r *layerRanker) markup(row ir.Row) int {
	r.sb.Reset()
	r.c.writeRow(&r.sb, row)
	return finalSVGBytes(r.sb.String(), r.c.config.Minify)
}

// groupSavings measures one cell group against the original row on its own;
// groups that would make the remaining row markup grow stay in the states,
// so an interval keeps only the groups that pay for themselves.
func (r *layerRanker) groupSavings(candidate *layerCandidate, y int, group []bool) int {
	colors := r.c.rec.Colors
	savings := -r.markup(cellsToRow(y, r.set.decoded[y][candidate.from], group, colors))
	for state := candidate.from; state <= candidate.to; state++ {
		keep := make([]bool, r.set.width)
		for col := range keep {
			keep[col] = !group[col] && cellVisible(r.set.decoded[y][state][col], colors)
		}
		before := r.rowCost(state, y)
		after := 0
		if remaining := cellsToRow(y, r.set.decoded[y][state], keep, colors); len(remaining.Runs) > 0 {
			after = r.markup(remaining)
			if before == rowReferenceBytes {
				after = min(after, rowReferenceBytes)
			}
		}
		savings += before - after
	}
	return savings
}

// cellGroups splits a row mask into groups of hoisted cells that are close
// enough to share one text element.
func cellGroups(mask []bool) [][]bool {
	var groups [][]bool
	var group []bool
	last := -1
	for col, hoisted := range mask {
		if !hoisted {
			continue
		}
		if group == nil || col-last > maxInertGap {
			group = make([]bool, len(mask))
			groups = append(groups, group)
		}
		group[col] = true
		last = col
	}
	return groups
}

// layerRowsAt returns the layer rows visible at the given time.
func (p *renderPlan) layerRowsAt(at time.Duration) []ir.Row {
	var rows []ir.Row
	for _, layer := range p.layers {
		if at >= layer.from && (layer.open || at < layer.to) {
			rows = append(rows, layer.rows...)
		}
	}
	return rows
}

func (c *canvas) layerVisibility(layer intervalLayer) []keyframePoint[bool] {
	points := []timelinePoint[bool]{{time: 0, state: layer.from == 0}}
	if layer.from > 0 {
		points = append(points, timelinePoint[bool]{time: layer.from, state: true})
	}
	if !layer.open {
		points = append(points, timelinePoint[bool]{time: layer.to, state: false})
	}
	return normalizeTimeline(c.plan.duration, points, func(a, b bool) bool { return a == b }).keyframes()
}

func layerVisibilityValue(visible bool) string {
	if visible {
		return "visible"
	}
	return "hidden"
}

func layerAnimationName(index int) string { return "l" + strconv.Itoa(index) }

func (c *canvas) writeLayers(w io.Writer) {
	for index := range c.plan.layers {
		c.writeLayer(w, index)
	}
}

func (c *canvas) writeLayer(w io.Writer, index int) {
	layer := &c.plan.layers[index]
	frames := c.layerVisibility(*layer)
	initial := ""
	if len(frames) > 0 && !frames[0].state {
		initial = ` visibility="hidden"`
	}
	if len(frames) > 1 && c.options.Animation == AnimationCSS {
		fmt.Fprintf(w, `<g style="animation:%s %s %s step-end%s"%s>`, layerAnimationName(index),
			animationDuration(c.plan.duration), c.loopCount(), c.scrollFiniteAnimationFill(), initial)
	} else {
		fmt.Fprintf(w, `<g%s>`, initial)
	}
	if len(frames) > 1 && c.options.Animation == AnimationSMIL {
		values := make([]string, len(frames))
		for i, frame := range frames {
			values[i] = layerVisibilityValue(frame.state)
		}
		c.writeSMILAnimation(w, "animate", []smilAttribute{
			{name: "attributeName", value: "visibility"},
			{name: "values", value: strings.Join(values, ";")},
			{name: "keyTimes", value: smilKeyTimes(frames)},
		})
	}
	for _, row := range layer.rows {
		c.writeRow(w, row)
	}
	fmt.Fprint(w, `</g>`)
}

func (c *canvas) generateLayerKeyframes(index int) string {
	frames := c.layerVisibility(c.plan.layers[index])
	if len(frames) <= 1 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "@keyframes %s{", layerAnimationName(index))
	for _, frame := range frames {
		fmt.Fprintf(&sb, "%s{visibility:%s}", frame.selector, layerVisibilityValue(frame.state))
	}
	sb.WriteString("}")
	return sb.String()
}

// layeredPlan is the test seam for the layer selection.
func layeredPlan(
	ctx context.Context,
	rec *ir.Recording,
	config *renderer.Config,
	options *Options,
) (renderPlan, error) {
	plan, err := buildSemanticPlan(ctx, rec, config.ShowCursor, options.MaxFPS, config.LoopCount)
	if err != nil {
		return renderPlan{}, err
	}
	return New(config, func(o *Options) { *o = *options }).selectIntervalLayers(ctx, rec, &plan)
}
