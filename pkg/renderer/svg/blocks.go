package svg

import (
	"slices"
	"strings"

	"github.com/mrmarble/termsvg/pkg/ir"
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

type blockOccurrence struct {
	state int // index into the state list
	start int // first row (inclusive)
	end   int // last row (exclusive)
	col   int // absolute column of the first row's first run
	y     int // absolute row of the first row
}

type blockCandidate struct {
	rows        []ir.Row // relative to the first row's column and row
	occurrences []blockOccurrence
	order       int
	users       []blockOccurrence
	anchor      blockOccurrence
	elements    int
	bodyBytes   int // markup bytes of the anchored body
	id          string
	dropped     bool
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

// shareRowBlocks selects profitable block definitions over the per-state row
// lists, appends them to the definitions, and replaces every covered row group
// by one block reference. It must run after whole-row interning so the bytes
// it compares are the ones that would otherwise be serialized.
func (c *canvas) shareRowBlocks(
	states [][]*renderedRow,
	definitions []*renderedRow,
	ids *xmlIDAllocator,
) (frames [][]*renderedRow, defs []*renderedRow) {
	frames, defs = states, definitions
	blockStates := c.buildBlockStates(frames)
	candidates := c.collectBlockCandidates(blockStates)
	if len(candidates) == 0 {
		return frames, defs
	}
	accepted := c.finalizeBlockIDs(blockStates, c.selectBlocks(blockStates, candidates, ids), ids)
	if len(accepted) == 0 {
		return frames, defs
	}
	replacements := make([][]blockReplacement, len(frames))
	for _, candidate := range accepted {
		def := c.blockDefinition(blockStates, candidate)
		defs = append(defs, def)
		for _, user := range candidate.users {
			replacements[user.state] = append(replacements[user.state], blockReplacement{
				start: user.start, end: user.end, item: c.blockReference(blockStates, candidate, def, user),
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

func (c *canvas) buildBlockStates(frames [][]*renderedRow) []blockState {
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
			state.prefixBytes[k+1] = state.prefixBytes[k] + c.renderedRowBytes(row)
			state.prefixElems[k+1] = state.prefixElems[k] + c.renderedRowItems(row)
		}
		states[i] = state
	}
	return states
}

// renderedRowBytes is what a row costs in a state today: its reference when
// it is interned, its markup otherwise.
func (c *canvas) renderedRowBytes(row *renderedRow) int {
	if row.id != "" {
		return c.segmentUseBytes(len(row.id), 0, 0)
	}
	return finalSVGBytes(row.svg, c.config.Minify)
}

// renderedRowItems is the number of XML elements a row contributes to a
// state today: one for a reference, its rendered elements otherwise.
func (c *canvas) renderedRowItems(row *renderedRow) int {
	if row.id != "" {
		return 1
	}
	return c.renderedElementCount(row)
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

func (c *canvas) collectBlockCandidates(states []blockState) []*blockCandidate {
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
			candidate = &blockCandidate{rows: relativeBlockRows(items, col, y), order: len(candidates)}
			byHash[hash] = append(byHash[hash], candidate)
			candidates = append(candidates, candidate)
		}
		candidate.occurrences = append(candidate.occurrences, blockOccurrence{
			state: state, start: start, end: end, col: col, y: y,
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

// selectBlocks greedily accepts candidates by their estimated total saving.
// Every occurrence whose rows are still unclaimed becomes a user, so a row
// joins at most one block and overlapping candidates resolve in favour of the
// larger win.
func (c *canvas) selectBlocks(
	states []blockState,
	candidates []*blockCandidate,
	ids *xmlIDAllocator,
) []*blockCandidate {
	estimatedIDLen := blockIDLength(ids, 0)
	estimates := make(map[*blockCandidate]int, len(candidates))
	for _, candidate := range candidates {
		c.chooseBlockAnchor(states, candidate, candidate.occurrences)
		estimates[candidate] = c.blockSavings(states, candidate, candidate.occurrences, estimatedIDLen)
	}
	slices.SortFunc(candidates, func(a, b *blockCandidate) int {
		if estimates[a] != estimates[b] {
			return estimates[b] - estimates[a]
		}
		return a.order - b.order
	})

	accepted := make([]*blockCandidate, 0)
	for _, candidate := range candidates {
		if estimates[candidate] <= 0 {
			break
		}
		users := make([]blockOccurrence, 0, len(candidate.occurrences))
		for _, occurrence := range candidate.occurrences {
			claimed := states[occurrence.state].claimed
			if segmentRunsFree(claimed, occurrence.start, occurrence.end) {
				users = append(users, occurrence)
				markSegmentRuns(claimed, occurrence.start, occurrence.end, true)
			}
		}
		if len(users) >= 2 {
			c.chooseBlockAnchor(states, candidate, users)
			if c.blockSavings(states, candidate, users, blockIDLength(ids, len(accepted))) > 0 {
				candidate.users = users
				accepted = append(accepted, candidate)
				continue
			}
		}
		releaseBlockUsers(states, users)
	}
	return accepted
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

func releaseBlockUsers(states []blockState, users []blockOccurrence) {
	for _, user := range users {
		markSegmentRuns(states[user.state].claimed, user.start, user.end, false)
	}
}

// finalizeBlockIDs assigns the shortest identifiers to the most referenced
// definitions and drops any candidate whose exact identifier cost makes it
// unprofitable. Dropping only shortens later identifiers, so one pass per drop
// converges quickly.
func (c *canvas) finalizeBlockIDs(
	states []blockState,
	accepted []*blockCandidate,
	ids *xmlIDAllocator,
) []*blockCandidate {
	slices.SortStableFunc(accepted, func(a, b *blockCandidate) int {
		return len(b.users) - len(a.users)
	})
	for {
		probe := *ids
		dropped := false
		for _, candidate := range accepted {
			id := probe.allocate()
			if c.blockSavings(states, candidate, candidate.users, len(id)) <= 0 {
				candidate.dropped, dropped = true, true
				releaseBlockUsers(states, candidate.users)
				continue
			}
			candidate.id = id
		}
		accepted = slices.DeleteFunc(accepted, func(candidate *blockCandidate) bool { return candidate.dropped })
		if !dropped {
			*ids = probe
			return accepted
		}
	}
}

// chooseBlockAnchor picks the most frequent absolute position among the
// users, the first occurrence on ties, and measures the body there.
func (c *canvas) chooseBlockAnchor(states []blockState, candidate *blockCandidate, users []blockOccurrence) {
	type position struct{ col, y int }
	counts := make(map[position]int, len(users))
	best, bestCount := position{}, 0
	for _, user := range users {
		at := position{user.col, user.y}
		counts[at]++
		if counts[at] > bestCount {
			best, bestCount = at, counts[at]
		}
	}
	for _, user := range users {
		if user.col == best.col && user.y == best.y {
			candidate.anchor = user
			break
		}
	}
	anchor := &states[candidate.anchor.state]
	candidate.bodyBytes = anchor.bytes(candidate.anchor.start, candidate.anchor.end)
	candidate.elements = anchor.elements(candidate.anchor.start, candidate.anchor.end)
}

// blockSavings is the exact byte change of sharing the candidate with the
// given identifier length: the definition costs its anchored body plus the
// wrapper, and every user trades what its rows cost today for one reference.
func (c *canvas) blockSavings(states []blockState, candidate *blockCandidate, users []blockOccurrence, idLen int) int {
	savings := -(candidate.bodyBytes + segmentDefinitionOverhead(idLen, candidate.elements))
	for _, user := range users {
		dx, dy := (user.col-candidate.anchor.col)*ColWidth, (user.y-candidate.anchor.y)*RowHeight
		savings += states[user.state].bytes(user.start, user.end) - c.segmentUseBytes(idLen, dx, dy)
	}
	return savings
}

// blockDefinition serializes the anchor rows as they are serialized today,
// references included, into one definition.
func (c *canvas) blockDefinition(states []blockState, candidate *blockCandidate) *renderedRow {
	anchor := candidate.anchor
	items := slices.Clone(states[anchor.state].items[anchor.start:anchor.end])
	def := &renderedRow{
		id: candidate.id, count: len(candidate.users), elements: candidate.elements,
		row: items[0].row, rows: blockRows(items), items: items,
	}
	var sb strings.Builder
	for _, item := range items {
		if item.id != "" {
			sb.WriteString(c.segmentUseMarkup(item.id, 0, 0))
			def.uses = append(def.uses, item.id)
			continue
		}
		sb.WriteString(item.svg)
		def.uses = append(def.uses, item.uses...)
	}
	def.svg = sb.String()
	def.definition = c.rowDefinition(def, def.id)
	return def
}

// blockReference replaces the covered rows of one user by a single reference
// to the definition, offset from the anchor position.
func (c *canvas) blockReference(
	states []blockState,
	candidate *blockCandidate,
	def *renderedRow,
	user blockOccurrence,
) *renderedRow {
	items := states[user.state].items[user.start:user.end]
	dx, dy := (user.col-candidate.anchor.col)*ColWidth, (user.y-candidate.anchor.y)*RowHeight
	return &renderedRow{
		row: items[0].row, rows: blockRows(items), svg: c.segmentUseMarkup(def.id, dx, dy),
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
