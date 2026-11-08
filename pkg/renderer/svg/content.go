package svg

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/mrmarble/termsvg/pkg/ir"
)

type preparedContent struct {
	frameKeyframes []keyframePoint[int]
	frameRows      [][]*renderedRow
	frameStateIDs  []string
	bands          []preparedBand
	rowDefs        []*renderedRow
	cost           preparedContentCost
}

type preparedBand struct {
	kind       preparedBandKind
	x          int
	y          int
	width      int
	height     int
	name       string
	keyframes  []keyframePoint[int]
	rows       [][]*renderedRow
	stateIDs   []string
	content    timeline[[]ir.Row]
	tapeRows   []*renderedRow
	tapeRaw    []ir.Row
	offsets    []keyframePoint[int]
	direction  int
	tapeHeight int
	track      *retainedRectTrack
}

type preparedBandKind uint8

const (
	bandSnapshot preparedBandKind = iota
	bandScrollTape
	bandRetainedRect
)

func (c *canvas) prepareContentContext(ctx context.Context) (preparedContent, error) {
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	if c.options.Layout == LayoutBands {
		return c.prepareBands(ctx)
	}
	if c.options.Layout == LayoutRegions {
		return c.prepareRegions(ctx)
	}
	if c.options.Layout == LayoutScroll {
		return c.prepareScroll(ctx)
	}
	keyframes, states := c.contentKeyframes()
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	frames, defs := c.collectRows(states)
	prepared := preparedContent{frameKeyframes: keyframes, frameRows: frames, rowDefs: defs}
	if c.options.FrameSwitch == FrameSwitchHref && len(keyframes) > 1 {
		prepared.frameStateIDs, _ = compactStateIDs(xmlIDIndexAfter(len(defs)), frames)
	}
	prepared.cost = buildPreparedContentCost(c, &prepared)
	return prepared, contextErr(ctx)
}

func (c *canvas) prepareRegions(ctx context.Context) (preparedContent, error) {
	regions := buildDynamicRegions(&c.plan, c.rec.Colors)
	optimized, err := c.optimizeDynamicRegions(ctx, regions)
	if err != nil {
		return preparedContent{}, err
	}
	if c.options.Primitives != PrimitiveRectTracks {
		return c.prepareLocalViewports(ctx, c.regionBands(optimized))
	}
	probe := *c
	probe.options.Primitives = PrimitiveSnapshots
	selected, err := probe.prepareLocalViewports(ctx, probe.regionBands(optimized))
	if err != nil {
		return preparedContent{}, err
	}
	for _, candidateRegions := range [][]dynamicRegion{optimized, regions} {
		candidate, err := c.prepareLocalViewports(ctx, c.regionBands(candidateRegions))
		if err != nil {
			return preparedContent{}, err
		}
		if candidate.cost.regionBytes < selected.cost.regionBytes {
			selected = candidate
		}
	}
	return selected, nil
}

func (c *canvas) regionBands(regions []dynamicRegion) []rowBand {
	bands := make([]rowBand, len(regions))
	for i, region := range regions {
		x, width, content := region.x, region.width, region.content
		if len(region.fallbackRows) == 0 {
			x = max(0, region.x-1)
			end := min(c.plan.width, region.x+region.width+1)
			width = end - x
			content = shiftRegionContent(content, region.x-x)
		}
		bands[i] = rowBand{x: x, y: region.y, width: width, height: region.height, content: content}
	}
	return bands
}

func shiftRegionContent(content timeline[[]ir.Row], columns int) timeline[[]ir.Row] {
	if columns == 0 {
		return content
	}
	shifted := content
	shifted.points = slices.Clone(content.points)
	for pointIndex := range shifted.points {
		shifted.points[pointIndex].state = slices.Clone(content.points[pointIndex].state)
		for rowIndex := range shifted.points[pointIndex].state {
			row := &shifted.points[pointIndex].state[rowIndex]
			row.Runs = slices.Clone(row.Runs)
			for runIndex := range row.Runs {
				row.Runs[runIndex].StartCol += columns
				row.Runs[runIndex].EndCol += columns
			}
		}
	}
	return shifted
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

func (c *canvas) prepareBands(ctx context.Context) (preparedContent, error) {
	bands := buildRowBands(&c.plan, c.plan.width, c.plan.height)
	return c.prepareLocalViewports(ctx, bands)
}

func (c *canvas) prepareScroll(ctx context.Context) (preparedContent, error) {
	bands := buildRowBands(&c.plan, c.plan.width, c.plan.height)
	probe := *c
	probe.options.Primitives = PrimitiveSnapshots
	snapshot, err := probe.prepareLocalViewports(ctx, bands)
	if err != nil {
		return preparedContent{}, err
	}
	retained, err := c.prepareLocalViewports(ctx, bands)
	if err != nil {
		return preparedContent{}, err
	}
	prepared := strictSmallestPrepared(snapshot, retained)
	for i, source := range bands {
		tape, ok := detectUpwardScrollTape(source, c.rec.Colors)
		if !ok {
			continue
		}
		candidateBands := slices.Clone(prepared.bands)
		candidateBands[i] = scrollTapeCandidateBand(&snapshot.bands[i], tape)
		candidate, err := c.materializeBands(ctx, candidateBands)
		if err != nil {
			return preparedContent{}, err
		}
		prepared = strictSmallestPrepared(prepared, candidate)
	}
	return prepared, nil
}

func scrollTapeCandidateBand(snapshot *preparedBand, tape scrollTape) preparedBand {
	band := *snapshot
	band.kind = bandScrollTape
	band.tapeRaw = tape.rows
	band.offsets = tape.offsets
	band.direction = 1
	band.tapeHeight = len(tape.rows)
	return band
}

func strictSmallestPrepared(candidates ...preparedContent) preparedContent {
	selected := 0
	for i := 1; i < len(candidates); i++ {
		if candidates[i].cost.regionBytes < candidates[selected].cost.regionBytes {
			selected = i
		}
	}
	return candidates[selected]
}

// exactBandReplacementDelta is the exact additive change in the complete
// prepared-content ledger when one band is replaced and every other band is
// held fixed. Shared definitions and styles are rebuilt in both ledgers, so
// their attributable byte changes are included without double counting.
func exactBandReplacementDelta(snapshot, tape preparedContentCost) int64 {
	return tape.definitions - snapshot.definitions + tape.styles - snapshot.styles + tape.active - snapshot.active
}

func scrollTapeWins(snapshot, tape preparedContentCost) bool {
	return exactBandReplacementDelta(snapshot, tape) < 0
}

func retainedRectWins(snapshot, retained preparedContentCost) bool {
	return exactBandReplacementDelta(snapshot, retained) < 0
}

func (c *canvas) prepareLocalViewports(ctx context.Context, bands []rowBand) (preparedContent, error) {
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	prepared := preparedContent{bands: make([]preparedBand, len(bands))}

	for i, band := range bands {
		keyframes, _ := contentKeyframesFor(band.content)
		prepared.bands[i] = preparedBand{
			kind:      bandSnapshot,
			x:         band.x,
			y:         band.y,
			width:     band.width,
			height:    band.height,
			keyframes: keyframes,
			content:   band.content,
		}
	}
	prepared, err := c.materializeBands(ctx, prepared.bands)
	if err != nil || c.options.Primitives != PrimitiveRectTracks {
		return prepared, err
	}
	for i, source := range bands {
		track, stripped, ok := c.retainedRectCandidate(source)
		if !ok {
			continue
		}
		candidateBands := slices.Clone(prepared.bands)
		candidateBands[i].kind = bandRetainedRect
		candidateBands[i].track = &track
		candidateBands[i].content = stripped
		candidateBands[i].keyframes, _ = contentKeyframesFor(stripped)
		candidate, err := c.materializeBands(ctx, candidateBands)
		if err != nil {
			return preparedContent{}, err
		}
		if retainedRectWins(prepared.cost, candidate.cost) {
			prepared = candidate
		}
	}
	return prepared, nil
}

//nolint:funlen // rows, state ids and animation names are allocated across all bands in order
func (c *canvas) materializeBands(ctx context.Context, bands []preparedBand) (preparedContent, error) {
	prepared := preparedContent{bands: slices.Clone(bands)}
	allStates := make([][]ir.Row, 0)
	stateOffsets := make([]int, len(bands)+1)
	for i := range prepared.bands {
		stateOffsets[i] = len(allStates)
		if prepared.bands[i].kind == bandScrollTape {
			allStates = append(allStates, prepared.bands[i].tapeRaw)
			continue
		}
		_, states := contentKeyframesFor(prepared.bands[i].content)
		allStates = append(allStates, states...)
	}
	stateOffsets[len(bands)] = len(allStates)

	frames, defs := c.collectRows(allStates)
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	prepared.rowDefs = defs
	// State identifiers continue the compact alphabet after the row
	// definitions so every href reference stays as short as possible.
	nextID := xmlIDIndexAfter(len(defs))
	for i := range prepared.bands {
		band := &prepared.bands[i]
		if band.kind == bandScrollTape {
			band.rows = nil
			band.tapeRows = frames[stateOffsets[i]]
			band.stateIDs = nil
			continue
		}
		band.rows = frames[stateOffsets[i]:stateOffsets[i+1]]
		if c.options.FrameSwitch == FrameSwitchHref && len(prepared.bands[i].keyframes) > 1 {
			prepared.bands[i].stateIDs, nextID = compactStateIDs(nextID, prepared.bands[i].rows)
		}
	}

	names := make(map[string]string)
	for i := range prepared.bands {
		frames := prepared.bands[i].keyframes
		if prepared.bands[i].kind == bandScrollTape {
			frames = prepared.bands[i].offsets
		}
		if len(frames) <= 1 {
			continue
		}
		signature := strconv.Itoa(int(prepared.bands[i].kind)) + "|" + keyframeSignature(frames, prepared.bands[i].width)
		name, ok := names[signature]
		if !ok {
			name = "b" + strconv.Itoa(len(names))
			names[signature] = name
		}
		prepared.bands[i].name = name
	}
	prepared.cost = buildPreparedContentCost(c, &prepared)
	return prepared, contextErr(ctx)
}

// compactStateIDs assigns identifiers to href-switched states, continuing the
// compact alphabet at the given allocator index. A state that is exactly one
// interned row is referenced through that row's identifier and needs no
// definition of its own, and every empty state shares one definition.
func compactStateIDs(next int, states [][]*renderedRow) (ids []string, following int) {
	ids = make([]string, len(states))
	allocator := xmlIDAllocator{next: next}
	empty := ""
	for i, rows := range states {
		switch {
		case len(rows) == 1 && rows[0].id != "":
			ids[i] = rows[0].id
		case len(rows) == 0 && empty != "":
			ids[i] = empty
		default:
			ids[i] = allocator.allocate()
			if len(rows) == 0 {
				empty = ids[i]
			}
		}
	}
	return ids, allocator.next
}

// ownsStateDefinition reports whether state i is serialized as its own
// definition rather than reusing a row definition or an earlier state.
func ownsStateDefinition(ids []string, states [][]*renderedRow, i int) bool {
	if len(states[i]) == 1 && states[i][0].id == ids[i] {
		return false
	}
	return !slices.Contains(ids[:i], ids[i])
}

// contentKeyframesFor maps a timeline onto its distinct states. A state that
// recurs later in the timeline, such as a region that alternates between two
// values, is defined once and referenced again from the keyframes.
func contentKeyframesFor(content timeline[[]ir.Row]) ([]keyframePoint[int], [][]ir.Row) {
	frames := content.keyframes(rowsEqual)
	states := make([][]ir.Row, 0, len(frames))
	byHash := make(map[uint64][]int, len(frames))
	out := make([]keyframePoint[int], len(frames))
	for i, frame := range frames {
		hash := rowsHash(frame.state)
		index := -1
		for _, candidate := range byHash[hash] {
			if rowsEqual(states[candidate], frame.state) {
				index = candidate
				break
			}
		}
		if index < 0 {
			index = len(states)
			states = append(states, frame.state)
			byHash[hash] = append(byHash[hash], index)
		}
		out[i] = keyframePoint[int]{selector: frame.selector, state: index}
	}
	if len(states) == 0 && len(content.points) > 0 {
		states = append(states, content.points[len(content.points)-1].state)
	}
	return out, states
}

func rowsHash(rows []ir.Row) uint64 {
	h := uint64(segmentHashOffset)
	for _, row := range rows {
		h = (h ^ semanticRowHash(row)) * segmentHashPrime
	}
	return h
}

func keyframeSignature(frames []keyframePoint[int], width int) string {
	var signature strings.Builder
	signature.WriteString(strconv.Itoa(width))
	signature.WriteByte('|')
	for _, frame := range frames {
		signature.WriteString(frame.selector)
		signature.WriteByte(':')
		signature.WriteString(strconv.Itoa(frame.state))
		signature.WriteByte(';')
	}
	return signature.String()
}
