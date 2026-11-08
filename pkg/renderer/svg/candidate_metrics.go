package svg

import (
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
)

// CandidateMetrics describes the serialized size and structural cost of one SVG candidate.
type CandidateMetrics struct {
	FinalBytes                            int64
	XMLNodes                              int
	DefinitionNodes                       int
	ActiveNodes                           int
	TextNodes                             int
	RectNodes                             int
	GroupNodes                            int
	UseNodes                              int
	AnimationNodes                        int
	AnimatedElements                      int
	StateDefinitions                      int
	MaxUseDepth                           int
	MaxTranslatedWidth                    int64
	MaxTranslatedArea                     int64
	LocalViewportCount                    int
	MaxViewportWidth                      int
	MaxViewportHeight                     int
	MaxViewportArea                       int64
	ScrollTapeCount                       int
	MaxTapeWidth                          int
	MaxTapeHeight                         int
	MaxTapeArea                           int64
	ScrollTransformAnimations             int
	RetainedPrimitiveCount                int
	GeometryAnimationNodes                int
	PaintPropertyAnimationNodes           int
	SourceActiveNodes                     int
	SourceDefinitionNodes                 int
	StaticUseShadowNodes                  uint64
	InitialAnimatedUseShadowNodes         uint64
	PeakAnimatedUseShadowNodes            uint64
	PeakInstantiatedNodes                 uint64
	DurationWeightedInstantiatedNodeNanos uint64
	UseTargetSwitches                     uint64
	PeakLiveNodeEstimate                  uint64
}

type definitionNode struct {
	nodes uint64
	uses  []string
}

type hrefMetricPoint struct {
	time time.Duration
	cost uint64
}

type hrefMetricSeries struct {
	points   []hrefMetricPoint
	switches uint64
}

func addPreparedMetrics(
	metrics *CandidateMetrics,
	content *preparedContent,
	options *Options,
	contentWidth, contentHeight int,
) {
	metrics.StateDefinitions = ownedStateDefinitions(content.frameStateIDs, content.frameRows)
	for i := range content.bands {
		band := &content.bands[i]
		metrics.StateDefinitions += ownedStateDefinitions(band.stateIDs, band.rows)
		metrics.LocalViewportCount++
		metrics.MaxViewportWidth = max(metrics.MaxViewportWidth, band.width*ColWidth)
		metrics.MaxViewportHeight = max(metrics.MaxViewportHeight, band.height*RowHeight)
		metrics.MaxViewportArea = max(metrics.MaxViewportArea, int64(band.width*ColWidth)*int64(band.height*RowHeight))
		if band.kind == bandScrollTape {
			metrics.ScrollTapeCount++
			metrics.MaxTapeWidth = max(metrics.MaxTapeWidth, band.width*ColWidth)
			metrics.MaxTapeHeight = max(metrics.MaxTapeHeight, band.tapeHeight*RowHeight)
			metrics.MaxTapeArea = max(metrics.MaxTapeArea, int64(band.width*ColWidth)*int64(band.tapeHeight*RowHeight))
			metrics.ScrollTransformAnimations++
			continue
		}
		if band.kind == bandRetainedRect {
			metrics.RetainedPrimitiveCount++
			metrics.GeometryAnimationNodes += band.track.geometryAnimationCount()
			metrics.PaintPropertyAnimationNodes += boolInt(band.track.fillChanges())
		}
		if options.FrameSwitch == FrameSwitchTranslate && len(band.keyframes) > 1 {
			addTranslatedSurface(metrics, band.width*ColWidth, band.height*RowHeight, len(band.rows))
		}
	}
	if !options.usesLocalViewports() && options.FrameSwitch == FrameSwitchTranslate && len(content.frameKeyframes) > 1 {
		addTranslatedSurface(metrics, contentWidth, contentHeight, len(content.frameRows))
	}
	if len(content.rowDefs) > 0 {
		metrics.MaxUseDepth = 1
	}
	if metrics.StateDefinitions > 0 {
		metrics.MaxUseDepth++
	}
}

func ownedStateDefinitions(ids []string, states [][]*renderedRow) int {
	count := 0
	for i := range ids {
		if ownsStateDefinition(ids, states, i) {
			count++
		}
	}
	return count
}

func addTranslatedSurface(metrics *CandidateMetrics, width, height, states int) {
	translatedWidth := int64(width * states)
	metrics.MaxTranslatedWidth = max(metrics.MaxTranslatedWidth, translatedWidth)
	metrics.MaxTranslatedArea = max(metrics.MaxTranslatedArea, translatedWidth*int64(height))
}

//nolint:gocognit,funlen // counts elements in the order the serializer writes them
func addStructuralMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) error {
	addMetric(metrics, false, "svg", 1)
	addMetric(metrics, false, "rect", 1)
	if c.config.ShowWindow {
		addMetric(metrics, false, "circle", len(c.config.Theme.WindowButtons))
	}
	addMetric(metrics, true, "defs", 1)
	addMetric(metrics, true, "clipPath", 1)
	addMetric(metrics, true, "rect", 1)
	addMetric(metrics, false, "g", 1)
	addMetric(metrics, false, "style", 1)

	for _, definition := range content.rowDefs {
		if c.renderedElementCount(definition) > 1 {
			addMetric(metrics, true, "g", 1)
		}
		renderedDefinition := *definition
		renderedDefinition.id = ""
		addRowMetrics(metrics, c, true, &renderedDefinition)
	}
	for _, static := range c.plan.staticRows {
		addRowMetrics(metrics, c, false, &renderedRow{row: static})
	}
	layerAnimations := 0
	for i := range c.plan.layers {
		addMetric(metrics, false, "g", 1)
		if len(c.layerVisibility(c.plan.layers[i])) > 1 {
			layerAnimations++
			if c.options.Animation == AnimationSMIL {
				addMetric(metrics, false, "animation", 1)
			}
		}
		for _, row := range c.plan.layers[i].rows {
			addRowMetrics(metrics, c, false, &renderedRow{row: row})
		}
	}

	for i := range content.frameStateIDs {
		if !ownsStateDefinition(content.frameStateIDs, content.frameRows, i) {
			continue
		}
		if c.stateNeedsWrapper(content.frameRows[i]) {
			addMetric(metrics, true, "g", 1)
		}
		addStateRowsMetrics(metrics, c, true, content.frameRows[i:i+1])
	}
	for bandIndex := range content.bands {
		band := &content.bands[bandIndex]
		for i := range band.stateIDs {
			if !ownsStateDefinition(band.stateIDs, band.rows, i) {
				continue
			}
			if c.stateNeedsWrapper(band.rows[i]) {
				addMetric(metrics, true, "g", 1)
			}
			addStateRowsMetrics(metrics, c, true, band.rows[i:i+1])
		}
	}

	animations := addActiveContentMetrics(metrics, c, content) + layerAnimations
	if c.plan.cursorEverVisible {
		addMetric(metrics, false, "g", 1)
		addMetric(metrics, false, "rect", 1)
		animations++ // The cursor rect always has the CSS blink animation.
		if c.options.Animation == AnimationCSS && len(c.cursorKeyframes()) > 1 {
			animations++
		}
		if c.options.Animation == AnimationSMIL && len(c.cursorKeyframes()) > 1 {
			cursorAnimations := cursorAnimationCount(c.cursorKeyframes())
			addMetric(metrics, false, "animation", cursorAnimations)
			if cursorAnimations > 0 {
				animations++
			}
		}
	}
	metrics.AnimatedElements = animations
	return addUseExpansionMetrics(metrics, c, content)
}

func recursiveDefinitionCost(
	graph map[string]definitionNode,
	id string,
	visiting map[string]bool,
) (cost uint64, depth int, err error) {
	if visiting[id] {
		return 0, 0, fmt.Errorf("cyclic SVG use definition %q", id)
	}
	node, ok := graph[id]
	if !ok {
		return 0, 0, fmt.Errorf("unknown SVG use definition %q", id)
	}
	visiting[id] = true
	cost, depth = node.nodes, 1
	for _, target := range node.uses {
		nested, nestedDepth, err := recursiveDefinitionCost(graph, target, visiting)
		if err != nil {
			return 0, 0, err
		}
		cost = saturatingAdd(cost, nested)
		depth = max(depth, nestedDepth+1)
	}
	delete(visiting, id)
	return cost, depth, nil
}

func saturatingAdd(a, b uint64) uint64 {
	if math.MaxUint64-a < b {
		return math.MaxUint64
	}
	return a + b
}

func saturatingMul(a, b uint64) uint64 {
	if a != 0 && b > math.MaxUint64/a {
		return math.MaxUint64
	}
	return a * b
}

//nolint:gocognit,funlen // both expansion modes read the use graph built here
func addUseExpansionMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) error {
	metrics.SourceActiveNodes, metrics.SourceDefinitionNodes = metrics.ActiveNodes, metrics.DefinitionNodes
	graph := make(map[string]definitionNode)
	for _, row := range content.rowDefs {
		elements := c.renderedElementCount(row)
		//nolint:gosec // element counts are non-negative.
		graph[row.id] = definitionNode{nodes: uint64(elements + boolInt(elements > 1)), uses: row.uses}
	}
	rowUses := func(rows []*renderedRow) (nodes uint64, ids []string) {
		for _, row := range rows {
			if row.id == "" {
				// Segment references are counted through ids below, one node each.
				//nolint:gosec // element counts are non-negative.
				nodes = saturatingAdd(nodes, uint64(c.renderedElementCount(row)-len(row.uses)))
				ids = append(ids, row.uses...)
				continue
			}
			ids = append(ids, row.id)
		}
		return
	}
	stateNode := func(rows []*renderedRow) definitionNode {
		nodes, uses := rowUses(rows)
		children := saturatingAdd(nodes, uint64(len(uses)))
		//nolint:gosec // boolInt returns 0 or 1.
		return definitionNode{nodes: saturatingAdd(children, uint64(boolInt(c.stateNeedsWrapper(rows)))), uses: uses}
	}
	for i, id := range content.frameStateIDs {
		if ownsStateDefinition(content.frameStateIDs, content.frameRows, i) {
			graph[id] = stateNode(content.frameRows[i])
		}
	}
	for bi := range content.bands {
		for i, id := range content.bands[bi].stateIDs {
			if ownsStateDefinition(content.bands[bi].stateIDs, content.bands[bi].rows, i) {
				graph[id] = stateNode(content.bands[bi].rows[i])
			}
		}
	}
	costs := make(map[string]uint64, len(graph))
	ids := make([]string, 0, len(graph))
	for id := range graph {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		cost, depth, err := recursiveDefinitionCost(graph, id, map[string]bool{})
		if err != nil {
			return err
		}
		costs[id] = cost
		metrics.MaxUseDepth = max(metrics.MaxUseDepth, depth)
	}
	expand := func(id string) uint64 { return costs[id] }
	definitionShadow := uint64(0)
	for _, node := range graph {
		for _, id := range node.uses {
			definitionShadow = saturatingAdd(definitionShadow, expand(id))
		}
	}
	metrics.StaticUseShadowNodes = definitionShadow
	var initial, peak, weighted, switches uint64
	if c.options.FrameSwitch == FrameSwitchHref {
		series := make([]hrefMetricSeries, 0, len(content.bands)+1)
		if len(content.frameStateIDs) > 0 {
			series = append(series, newHrefMetricSeries(content.frameKeyframes, content.frameStateIDs, c.plan.duration, expand))
		}
		for i := range content.bands {
			band := &content.bands[i]
			if len(band.stateIDs) > 0 {
				series = append(series, newHrefMetricSeries(band.keyframes, band.stateIDs, c.plan.duration, expand))
			}
		}
		initial, peak, weighted, switches = combineHrefMetricSeries(series, c.plan.duration)
	} else {
		for _, states := range append([][][]*renderedRow{content.frameRows}, bandRows(content.bands)...) {
			for _, rows := range states {
				for _, row := range rows {
					if row.id != "" {
						peak = saturatingAdd(peak, expand(row.id))
						continue
					}
					for _, id := range row.uses {
						peak = saturatingAdd(peak, expand(id))
					}
				}
			}
		}
		initial = peak
		weighted = saturatingMul(peak, uint64(max(time.Duration(0), c.plan.duration)))
	}
	metrics.InitialAnimatedUseShadowNodes, metrics.PeakAnimatedUseShadowNodes = initial, peak
	metrics.UseTargetSwitches = switches
	shadows := saturatingAdd(definitionShadow, peak)
	//nolint:gosec // node counts are non-negative.
	xmlNodes, activeNodes := uint64(metrics.XMLNodes), uint64(metrics.ActiveNodes)
	metrics.PeakInstantiatedNodes = saturatingAdd(xmlNodes, shadows)
	metrics.PeakLiveNodeEstimate = saturatingAdd(activeNodes, peak)
	duration := uint64(max(time.Duration(0), c.plan.duration))
	base := saturatingAdd(xmlNodes, definitionShadow)
	metrics.DurationWeightedInstantiatedNodeNanos = saturatingAdd(saturatingMul(base, duration), weighted)
	return nil
}

func newHrefMetricSeries(
	frames []keyframePoint[int],
	ids []string,
	duration time.Duration,
	expand func(string) uint64,
) hrefMetricSeries {
	series := hrefMetricSeries{points: make([]hrefMetricPoint, 0, len(frames))}
	lastState := -1
	for _, frame := range frames {
		if frame.state < 0 || frame.state >= len(ids) {
			continue
		}
		if lastState >= 0 && frame.state != lastState {
			series.switches++
		}
		lastState = frame.state
		series.points = append(series.points, hrefMetricPoint{
			time: keyframeSelectorTime(frame.selector, duration),
			cost: expand(ids[frame.state]),
		})
	}
	return series
}

func combineHrefMetricSeries(
	series []hrefMetricSeries,
	duration time.Duration,
) (initial, peak, weighted, switches uint64) {
	boundaries := []time.Duration{0, duration}
	for _, sequence := range series {
		switches = saturatingAdd(switches, sequence.switches)
		for _, point := range sequence.points {
			boundaries = append(boundaries, point.time)
		}
	}
	slices.Sort(boundaries)
	boundaries = slices.Compact(boundaries)
	costAt := func(sequence hrefMetricSeries, at time.Duration) uint64 {
		cost := uint64(0)
		for _, point := range sequence.points {
			if point.time > at {
				break
			}
			cost = point.cost
		}
		return cost
	}
	for i, boundary := range boundaries {
		live := uint64(0)
		for _, sequence := range series {
			live = saturatingAdd(live, costAt(sequence, boundary))
		}
		if i == 0 {
			initial = live
		}
		peak = max(peak, live)
		if i+1 < len(boundaries) {
			weighted = saturatingAdd(weighted, saturatingMul(live, uint64(max(time.Duration(0), boundaries[i+1]-boundary))))
		}
	}
	return
}

func keyframeSelectorTime(selector string, duration time.Duration) time.Duration {
	decimal := strings.TrimSuffix(selector, "%")
	parts := strings.SplitN(decimal, ".", 2)
	digits, scale := parts[0], int64(1)
	if len(parts) == 2 {
		digits += parts[1]
		for range len(parts[1]) {
			scale *= 10
		}
	}
	percent, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(int64(duration)), big.NewInt(percent))
	numerator.Quo(numerator, big.NewInt(100*scale))
	return time.Duration(numerator.Int64())
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func bandRows(bands []preparedBand) [][][]*renderedRow {
	out := make([][][]*renderedRow, len(bands))
	for i := range bands {
		out[i] = bands[i].rows
	}
	return out
}

func addMetric(metrics *CandidateMetrics, definition bool, kind string, count int) {
	if count == 0 {
		return
	}
	metrics.XMLNodes += count
	if definition {
		metrics.DefinitionNodes += count
	} else {
		metrics.ActiveNodes += count
	}
	switch kind {
	case "text":
		metrics.TextNodes += count
	case "rect":
		metrics.RectNodes += count
	case "g":
		metrics.GroupNodes += count
	case "use":
		metrics.UseNodes += count
	case "animation":
		metrics.AnimationNodes += count
	}
}

func addRowMetrics(metrics *CandidateMetrics, c *canvas, definition bool, rendered *renderedRow) {
	if rendered.id != "" {
		addMetric(metrics, definition, "use", 1)
		return
	}
	addMetric(metrics, definition, "use", len(rendered.uses))
	for _, row := range rendered.paintRows() {
		addMetric(metrics, definition, "rect", len(c.backgroundSpans(row)))
		for _, run := range row.Runs {
			if shouldRenderText(run) {
				addMetric(metrics, definition, "text", 1)
			}
		}
	}
}

func addStateRowsMetrics(
	metrics *CandidateMetrics,
	c *canvas,
	definition bool,
	states [][]*renderedRow,
) {
	for _, rows := range states {
		for _, rendered := range rows {
			addRowMetrics(metrics, c, definition, rendered)
		}
	}
}

func addActiveContentMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) int {
	switch {
	case c.options.usesLocalViewports():
		return addLocalViewportMetrics(metrics, c, content)
	case len(content.frameKeyframes) <= 1:
		addStateRowsMetrics(metrics, c, false, content.frameRows)
		return 0
	case c.options.FrameSwitch == FrameSwitchHref:
		addMetric(metrics, false, "use", 1)
		addMetric(metrics, false, "animation", 1)
		return 1
	default:
		addMetric(metrics, false, "g", 1+len(content.frameRows))
		animations := 1
		if c.options.Animation == AnimationSMIL {
			addMetric(metrics, false, "animation", 1)
		}
		addStateRowsMetrics(metrics, c, false, content.frameRows)
		return animations
	}
}

func addLocalViewportMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) int {
	animations := 0
	for i := range content.bands {
		band := &content.bands[i]
		if band.kind != bandSnapshot || c.options.FrameSwitch != FrameSwitchHref || len(band.keyframes) <= 1 {
			addMetric(metrics, false, "svg", 1)
		}
		if band.kind == bandScrollTape {
			addMetric(metrics, false, "g", 1)
			if c.options.Animation == AnimationSMIL {
				addMetric(metrics, false, "animation", 1)
			}
			addStateRowsMetrics(metrics, c, false, [][]*renderedRow{band.tapeRows})
			animations++
			continue
		}
		if band.kind == bandRetainedRect {
			addMetric(metrics, false, "rect", 1)
			animationCount := band.track.animationCount()
			addMetric(metrics, false, "animation", animationCount)
			if animationCount > 0 {
				animations++
			}
		}
		if len(band.keyframes) <= 1 {
			addStateRowsMetrics(metrics, c, false, band.rows)
			continue
		}
		if c.options.FrameSwitch == FrameSwitchHref {
			addMetric(metrics, false, "use", 1)
			addMetric(metrics, false, "animation", 1)
			animations++
			continue
		}
		addMetric(metrics, false, "g", 1+len(band.rows))
		animations++
		if c.options.Animation == AnimationSMIL {
			addMetric(metrics, false, "animation", 1)
		}
		addStateRowsMetrics(metrics, c, false, band.rows)
	}
	return animations
}

func cursorAnimationCount(frames []keyframePoint[ir.Cursor]) int {
	position, visibility := false, false
	for i := 1; i < len(frames); i++ {
		position = position || frames[i-1].state.Col != frames[i].state.Col ||
			frames[i-1].state.Row != frames[i].state.Row
		visibility = visibility || frames[i-1].state.Visible != frames[i].state.Visible
	}
	count := 0
	if position {
		count++
	}
	if visibility {
		count++
	}
	return count
}
