package svg

import (
	"slices"
	"strings"

	"github.com/mrmarble/termsvg/internal/ir"
)

// Block sharing factors recurring groups of consecutive rows out of the
// states, one level above segment sharing. A box that moves across the
// screen, a panel that is redrawn at another position, or a table fragment
// that recurs after a redraw is serialized once as a definition and referenced
// with one <use> per occurrence that carries the positional offset. A group is
// made of the rows exactly as they are serialized today, whole-row and segment
// references included, and its items keep their order inside the definition,
// so the paint order never changes.
//
// Content equality is position independent: rows match when their runs match
// relative to the group's first run, and groups match when their rows match
// pairwise and the row and column distances between consecutive rows are
// equal. Definitions are anchored at the most frequent absolute position so
// those references need no offset.

// blockCandidate shares occurrence units that are indexes into the state list
// and ranges that are row indexes.
type blockCandidate struct {
	shareCandidate
	rows []ir.Row // relative to the first row's column and row
}

// blockState caches what every row of one state costs as it is serialized
// today, so occurrence bytes and element counts are prefix-sum lookups.
type blockState struct {
	items       []*renderedRow
	hashes      []uint64 // position-independent content hash of each row
	cols        []int    // column of each row's first run
	prefixBytes []int
	prefixElems []int
	claimed     []bool
}

type blockRowKey struct {
	hash uint64
	col  int
}

type blockReplacement struct {
	start, end int
	item       *renderedRow
}

const (
	// minBlockRows and maxBlockRows bound the group length. Longer recurring
	// regions are covered by several blocks.
	minBlockRows = 2
	maxBlockRows = 12
)

func (s *blockState) bytes(start, end int) int { return s.prefixBytes[end] - s.prefixBytes[start] }

func (s *blockState) elements(start, end int) int { return s.prefixElems[end] - s.prefixElems[start] }

// blockUnits measures row ranges of the states for the sharing engine.
type blockUnits []blockState

func (u blockUnits) Bytes(unit, start, end int) int    { return u[unit].bytes(start, end) }
func (u blockUnits) Elements(unit, start, end int) int { return u[unit].elements(start, end) }
func (u blockUnits) Claimed(unit int) []bool           { return u[unit].claimed }

// shareRowBlocks selects profitable block definitions over the per-state row
// lists, appends them to the definitions, and replaces every covered row group
// by one block reference. It must run after whole-row interning so the bytes
// it compares are the ones that would otherwise be serialized.
func (rc rowCollector) shareRowBlocks(
	states [][]*renderedRow,
	definitions []*renderedRow,
	ids *xmlIDAllocator,
) (frames [][]*renderedRow, defs []*renderedRow) {
	frames, defs = states, definitions
	blockStates := rc.buildBlockStates(frames)
	candidates := collectBlockCandidates(blockStates)
	if len(candidates) == 0 {
		return frames, defs
	}
	units := blockUnits(blockStates)
	selected := selectShared(rc.rows.minify, units, candidates, blockIDLength(ids, 0),
		func(accepted int) int { return blockIDLength(ids, accepted) })
	accepted := finalizeShared(rc.rows.minify, units, selected, ids)
	if len(accepted) == 0 {
		return frames, defs
	}
	replacements := make([][]blockReplacement, len(frames))
	for _, candidate := range accepted {
		def := rc.blockDefinition(blockStates, candidate)
		defs = append(defs, def)
		for _, user := range candidate.users {
			replacements[user.unit] = append(replacements[user.unit], blockReplacement{
				start: user.start, end: user.end, item: blockReference(rc.rows.minify, blockStates, candidate, def, user),
			})
		}
	}
	for i, list := range replacements {
		if len(list) == 0 {
			continue
		}
		slices.SortFunc(list, func(a, b blockReplacement) int { return a.start - b.start })
		rows := make([]*renderedRow, 0, len(frames[i]))
		next := 0
		for _, replacement := range list {
			rows = append(rows, frames[i][next:replacement.start]...)
			rows = append(rows, replacement.item)
			next = replacement.end
		}
		frames[i] = append(rows, frames[i][next:]...)
	}
	return frames, defs
}

func (rc rowCollector) buildBlockStates(frames [][]*renderedRow) []blockState {
	states := make([]blockState, len(frames))
	keys := make(map[*renderedRow]blockRowKey)
	for i, rows := range frames {
		n := len(rows)
		state := blockState{
			items: rows, hashes: make([]uint64, n), cols: make([]int, n),
			prefixBytes: make([]int, n+1), prefixElems: make([]int, n+1), claimed: make([]bool, n),
		}
		for k, row := range rows {
			key, ok := keys[row]
			if !ok {
				key = blockRowKey{hash: blockRowHash(row.row), col: firstRunCol(row.row)}
				keys[row] = key
			}
			state.hashes[k], state.cols[k] = key.hash, key.col
			state.prefixBytes[k+1] = state.prefixBytes[k] + renderedRowBytes(rc.rows.minify, row)
			state.prefixElems[k+1] = state.prefixElems[k] + rc.renderedRowItems(row)
		}
		states[i] = state
	}
	return states
}

// renderedRowBytes is what a row costs in a state today: its reference when
// it is interned, its markup otherwise.
func renderedRowBytes(minify bool, row *renderedRow) int {
	if row.id != "" {
		return segmentUseBytes(minify, len(row.id), 0, 0)
	}
	return finalSVGBytes(row.svg, minify)
}

// renderedRowItems is the number of XML elements a row contributes to a
// state today: one for a reference, its rendered elements otherwise.
func (rc rowCollector) renderedRowItems(row *renderedRow) int {
	if row.id != "" {
		return 1
	}
	return rc.rows.renderedElementCount(row)
}

// blockRowHash hashes a row independently of its absolute position.
func blockRowHash(row ir.Row) uint64 {
	h := mixSegmentHash(segmentHashOffset, len(row.Runs))
	for k, run := range row.Runs {
		if k > 0 {
			h = mixSegmentHash(h, run.StartCol-row.Runs[k-1].StartCol)
		}
		h = (h ^ segmentRunHash(run)) * segmentHashPrime
	}
	return h
}

func firstRunCol(row ir.Row) int {
	if len(row.Runs) == 0 {
		return 0
	}
	return row.Runs[0].StartCol
}

// visitBlockRanges enumerates every group of consecutive rows within the
// length bounds, rolling a hash that mixes each row's content with its row
// and column distance from the previous one.
func visitBlockRanges(states []blockState, visitor func(state, start, end int, hash uint64)) {
	for stateIndex := range states {
		s := &states[stateIndex]
		for start := range s.items {
			h := uint64(segmentHashOffset)
			for end := start + 1; end <= len(s.items) && end-start <= maxBlockRows; end++ {
				k := end - 1
				if k > start {
					h = mixSegmentHash(h, s.items[k].row.Y-s.items[k-1].row.Y)
					h = mixSegmentHash(h, s.cols[k]-s.cols[k-1])
				}
				h = (h ^ s.hashes[k]) * segmentHashPrime
				if end-start >= minBlockRows && s.bytes(start, end) > 0 {
					visitor(stateIndex, start, end, h)
				}
			}
		}
	}
}

func collectBlockCandidates(states []blockState) []*blockCandidate {
	counts := make(map[uint64]int)
	visitBlockRanges(states, func(_, _, _ int, hash uint64) { counts[hash]++ })

	byHash := make(map[uint64][]*blockCandidate)
	candidates := make([]*blockCandidate, 0)
	visitBlockRanges(states, func(state, start, end int, hash uint64) {
		if counts[hash] < 2 {
			return
		}
		items := states[state].items[start:end]
		col, y := states[state].cols[start], items[0].row.Y
		var candidate *blockCandidate
		for _, existing := range byHash[hash] {
			if blockRowsEqual(existing.rows, items, col, y) {
				candidate = existing
				break
			}
		}
		if candidate == nil {
			candidate = &blockCandidate{
				shareCandidate: shareCandidate{order: len(candidates)}, rows: relativeBlockRows(items, col, y),
			}
			byHash[hash] = append(byHash[hash], candidate)
			candidates = append(candidates, candidate)
		}
		candidate.occurrences = append(candidate.occurrences, occurrence{
			unit: state, start: start, end: end, col: col, y: y,
		})
	})
	return slices.DeleteFunc(candidates, func(candidate *blockCandidate) bool {
		return len(candidate.occurrences) < 2
	})
}

// relativeBlockRows shifts the rows of a group so its first row's first run
// starts at column 0 of row 0. Run end columns are normalized on the way.
func relativeBlockRows(items []*renderedRow, col, y int) []ir.Row {
	rows := make([]ir.Row, len(items))
	for i, item := range items {
		rows[i] = ir.Row{Y: item.row.Y - y, Runs: make([]ir.TextRun, len(item.row.Runs))}
		for k, run := range item.row.Runs {
			rows[i].Runs[k] = ir.TextRun{
				Text: run.Text, StartCol: run.StartCol - col, EndCol: runEndCol(run) - col, Attrs: run.Attrs,
			}
		}
	}
	return rows
}

func blockRowsEqual(relative []ir.Row, items []*renderedRow, col, y int) bool {
	if len(relative) != len(items) {
		return false
	}
	for i, item := range items {
		if relative[i].Y != item.row.Y-y || len(relative[i].Runs) != len(item.row.Runs) {
			return false
		}
		for k, run := range item.row.Runs {
			want := relative[i].Runs[k]
			if want.Text != run.Text || want.StartCol != run.StartCol-col ||
				want.EndCol != runEndCol(run)-col || want.Attrs != run.Attrs {
				return false
			}
		}
	}
	return true
}

// blockIDLength is the length of the identifier the allocator would hand out
// after index further block definitions.
func blockIDLength(ids *xmlIDAllocator, index int) int {
	probe := *ids
	for range index {
		probe.allocate()
	}
	return len(probe.allocate())
}

// blockDefinition serializes the anchor rows as they are serialized today,
// references included, into one definition.
func (rc rowCollector) blockDefinition(states []blockState, candidate *blockCandidate) *renderedRow {
	anchor := candidate.anchor
	items := slices.Clone(states[anchor.unit].items[anchor.start:anchor.end])
	def := &renderedRow{
		id: candidate.id, count: len(candidate.users), elements: candidate.elements,
		row: items[0].row, rows: blockRows(items), items: items,
	}
	var sb strings.Builder
	for _, item := range items {
		if item.id != "" {
			sb.WriteString(segmentUseMarkup(rc.rows.minify, item.id, 0, 0))
			def.uses = append(def.uses, item.id)
			continue
		}
		sb.WriteString(item.svg)
		def.uses = append(def.uses, item.uses...)
	}
	def.svg = sb.String()
	def.definition = rc.rows.rowDefinition(def, def.id)
	return def
}

// blockReference replaces the covered rows of one user by a single reference
// to the definition, offset from the anchor position.
func blockReference(
	minify bool,
	states []blockState,
	candidate *blockCandidate,
	def *renderedRow,
	user occurrence,
) *renderedRow {
	items := states[user.unit].items[user.start:user.end]
	dx, dy := (user.col-candidate.anchor.col)*ColWidth, (user.y-candidate.anchor.y)*RowHeight
	return &renderedRow{
		row: items[0].row, rows: blockRows(items), svg: segmentUseMarkup(minify, def.id, dx, dy),
		elements: 1, uses: []string{def.id},
	}
}

func blockRows(items []*renderedRow) []ir.Row {
	rows := make([]ir.Row, len(items))
	for i, item := range items {
		rows[i] = item.row
	}
	return rows
}
