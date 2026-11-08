package svg

import (
	"slices"
	"strings"

	"github.com/mrmarble/termsvg/pkg/ir"
)

// Segment sharing factors repeated run sequences out of otherwise distinct
// rows. A shell prompt that is redrawn on every keystroke, a table fragment
// that recurs on several rows, or a line that scrolls through different
// vertical positions is serialized once as a definition and referenced with
// <use> elements that carry the positional offset. Definitions are anchored at
// the most frequent absolute position so those references need no offset.
//
// Whole-row interning still runs afterwards on the resulting markup, so rows
// that repeat verbatim at one position keep their existing references.

type segmentOccurrence struct {
	row   int // index into the unique row list
	start int // first run (inclusive)
	end   int // last run (exclusive)
	col   int // absolute column of the first run
	y     int // absolute row
}

type segmentCandidate struct {
	runs        []ir.TextRun // normalized to column 0
	occurrences []segmentOccurrence
	order       int
	users       []segmentOccurrence
	anchorCol   int
	anchorY     int
	elements    int
	inlineBytes int // markup bytes of the anchored body
	def         *renderedRow
	dropped     bool
}

// rowSegmentUse records one accepted reference inside a unique row.
type rowSegmentUse struct {
	start, end int
	def        *renderedRow
	dx, dy     int
}

// rowMarkup holds the per-run markup of one unique row so segment byte
// accounting and the final serialization agree exactly.
type rowMarkup struct {
	row       ir.Row
	texts     []string
	spans     []backgroundSpan
	rects     []string
	boundary  []bool   // boundary[k] is true when a segment may start at run k
	byteAt    []int    // markup bytes attributed to run k (its text and any span starting there)
	elemAt    []int    // elements attributed to run k
	prefixLen []int    // prefix sums of byteAt
	prefixEl  []int    // prefix sums of elemAt
	runHash   []uint64 // position-independent hash of run k
	gapHash   []uint64 // hash of the column distance between run k-1 and run k
}

const (
	// maxSegmentRuns bounds the candidate enumeration to O(runs × maxSegmentRuns)
	// per row. Prompts and table fragments are far shorter than this.
	maxSegmentRuns = 64

	segmentHashOffset = 14695981039346656037
	segmentHashPrime  = 1099511628211
)

func (c *canvas) buildRowMarkup(row ir.Row) rowMarkup {
	n := len(row.Runs)
	m := rowMarkup{
		row: row, texts: make([]string, n), boundary: make([]bool, n+1),
		byteAt: make([]int, n), elemAt: make([]int, n), prefixLen: make([]int, n+1), prefixEl: make([]int, n+1),
		runHash: make([]uint64, n), gapHash: make([]uint64, n),
	}
	for k, run := range row.Runs {
		m.runHash[k] = segmentRunHash(run)
		if k > 0 {
			m.gapHash[k] = mixSegmentHash(segmentHashOffset, run.StartCol-row.Runs[k-1].StartCol)
		}
	}
	m.spans = c.backgroundSpans(row)
	m.rects = make([]string, len(m.spans))
	for i := range m.boundary {
		m.boundary[i] = true
	}
	var sb strings.Builder
	for i, span := range m.spans {
		sb.Reset()
		c.writeBackgroundSpan(&sb, row.Y, span)
		m.rects[i] = sb.String()
		m.byteAt[span.firstRun] += finalSVGBytes(m.rects[i], c.config.Minify)
		m.elemAt[span.firstRun]++
		// A merged background rectangle must not be split across a segment
		// boundary: it would change both paint order and rectangle count.
		for k := span.firstRun + 1; k <= span.lastRun; k++ {
			m.boundary[k] = false
		}
	}
	for k, run := range row.Runs {
		sb.Reset()
		c.writeTextRun(&sb, run, row.Y)
		m.texts[k] = sb.String()
		if m.texts[k] != "" {
			m.byteAt[k] += finalSVGBytes(m.texts[k], c.config.Minify)
			m.elemAt[k]++
		}
	}
	for k := range n {
		m.prefixLen[k+1] = m.prefixLen[k] + m.byteAt[k]
		m.prefixEl[k+1] = m.prefixEl[k] + m.elemAt[k]
	}
	return m
}

func (m *rowMarkup) bytes(start, end int) int { return m.prefixLen[end] - m.prefixLen[start] }
func (m *rowMarkup) elements(start, end int) int {
	return m.prefixEl[end] - m.prefixEl[start]
}

// render serializes the row with the accepted segment references. Rectangles
// of the inline runs keep their place before every text element, so a row
// without references is byte-identical to the plain serialization.
func (m *rowMarkup) render(c *canvas, uses []rowSegmentUse) (svg string, elements int, inline ir.Row) {
	var sb strings.Builder
	inline = ir.Row{Y: m.row.Y}
	covered := func(run int) bool {
		for _, use := range uses {
			if run >= use.start && run < use.end {
				return true
			}
		}
		return false
	}
	for i, span := range m.spans {
		if !covered(span.firstRun) {
			sb.WriteString(m.rects[i])
			elements++
		}
	}
	next := 0
	for _, use := range uses {
		for ; next < use.start; next++ {
			sb.WriteString(m.texts[next])
			inline.Runs = append(inline.Runs, m.row.Runs[next])
			if m.texts[next] != "" {
				elements++
			}
		}
		sb.WriteString(c.segmentUseMarkup(use.def.id, use.dx, use.dy))
		elements++
		next = use.end
	}
	for ; next < len(m.texts); next++ {
		sb.WriteString(m.texts[next])
		inline.Runs = append(inline.Runs, m.row.Runs[next])
		if m.texts[next] != "" {
			elements++
		}
	}
	return sb.String(), elements, inline
}

func (c *canvas) segmentUseMarkup(id string, dx, dy int) string {
	var sb strings.Builder
	sb.WriteString(`<use href="#`)
	sb.WriteString(id)
	sb.WriteByte('"')
	if dx != 0 {
		sb.WriteString(` x="`)
		sb.WriteString(c.xmlInt(dx))
		sb.WriteByte('"')
	}
	if dy != 0 {
		sb.WriteString(` y="`)
		sb.WriteString(c.xmlInt(dy))
		sb.WriteByte('"')
	}
	sb.WriteString(`/>`)
	return sb.String()
}

func (c *canvas) segmentUseBytes(idLen, dx, dy int) int {
	bytes := len(`<use href="#`) + idLen + len(`"/>`)
	if dx != 0 {
		bytes += len(` x=""`) + len(c.xmlInt(dx))
	}
	if dy != 0 {
		bytes += len(` y=""`) + len(c.xmlInt(dy))
	}
	return bytes
}

func segmentDefinitionOverhead(idLen, elements int) int {
	if elements == 1 {
		return len(` id=""`) + idLen
	}
	return len(`<g id=""></g>`) + idLen
}

func normalizedSegmentRuns(runs []ir.TextRun) []ir.TextRun {
	base := runs[0].StartCol
	out := make([]ir.TextRun, len(runs))
	for i, run := range runs {
		out[i] = ir.TextRun{Text: run.Text, StartCol: run.StartCol - base, EndCol: runEndCol(run) - base, Attrs: run.Attrs}
	}
	return out
}

func mixSegmentHash(h uint64, value int) uint64 {
	//nolint:gosec // convert signed values to stable two's-complement hash words.
	return (h ^ uint64(value)) * segmentHashPrime
}

// segmentRunHash hashes a run independently of its absolute position. The
// distance to the preceding run is mixed in separately while extending a
// candidate, so equal run sequences hash equally wherever they occur.
func segmentRunHash(run ir.TextRun) uint64 {
	h := uint64(segmentHashOffset)
	for i := range len(run.Text) {
		h = (h ^ uint64(run.Text[i])) * segmentHashPrime
	}
	h = mixSegmentHash(h, len(run.Text))
	h = mixSegmentHash(h, runEndCol(run)-run.StartCol)
	h = mixSegmentHash(h, int(run.Attrs.FG))
	h = mixSegmentHash(h, int(run.Attrs.BG))
	flags := 0
	for i, set := range []bool{run.Attrs.Bold, run.Attrs.Italic, run.Attrs.Underline, run.Attrs.Dim} {
		if set {
			flags |= 1 << i
		}
	}
	return mixSegmentHash(h, flags)
}

// shareRowSegments selects profitable segment definitions for the unique rows
// and returns the references each row should emit together with the
// definitions in identifier order.
func (c *canvas) shareRowSegments(markup []rowMarkup, ids *xmlIDAllocator) ([][]rowSegmentUse, []*renderedRow) {
	candidates := c.collectSegmentCandidates(markup)
	if len(candidates) == 0 {
		return make([][]rowSegmentUse, len(markup)), nil
	}
	claimed := make([][]bool, len(markup))
	for i := range markup {
		claimed[i] = make([]bool, len(markup[i].row.Runs))
	}
	accepted := c.selectSegments(markup, candidates, claimed)
	accepted = c.finalizeSegmentIDs(markup, accepted, claimed, ids)

	uses := make([][]rowSegmentUse, len(markup))
	defs := make([]*renderedRow, 0, len(accepted))
	for _, candidate := range accepted {
		candidate.def = c.segmentDefinition(candidate)
		defs = append(defs, candidate.def)
		for _, user := range candidate.users {
			uses[user.row] = append(uses[user.row], rowSegmentUse{
				start: user.start, end: user.end, def: candidate.def,
				dx: (user.col - candidate.anchorCol) * ColWidth, dy: (user.y - candidate.anchorY) * RowHeight,
			})
		}
	}
	for i := range uses {
		slices.SortFunc(uses[i], func(a, b rowSegmentUse) int { return a.start - b.start })
	}
	return uses, defs
}

func (c *canvas) collectSegmentCandidates(markup []rowMarkup) []*segmentCandidate {
	counts := make(map[uint64]int)
	visitSegmentRanges(markup, func(_, _, _ int, hash uint64) { counts[hash]++ })

	byHash := make(map[uint64][]*segmentCandidate)
	candidates := make([]*segmentCandidate, 0)
	visitSegmentRanges(markup, func(row, start, end int, hash uint64) {
		if counts[hash] < 2 {
			return
		}
		runs := markup[row].row.Runs[start:end]
		var candidate *segmentCandidate
		for _, existing := range byHash[hash] {
			if segmentRunsEqual(existing.runs, runs) {
				candidate = existing
				break
			}
		}
		if candidate == nil {
			candidate = &segmentCandidate{runs: normalizedSegmentRuns(runs), order: len(candidates)}
			byHash[hash] = append(byHash[hash], candidate)
			candidates = append(candidates, candidate)
		}
		candidate.occurrences = append(candidate.occurrences, segmentOccurrence{
			row: row, start: start, end: end, col: runs[0].StartCol, y: markup[row].row.Y,
		})
	})
	candidates = slices.DeleteFunc(candidates, func(candidate *segmentCandidate) bool {
		first := candidate.occurrences[0]
		return len(candidate.occurrences) < 2 || markup[first.row].bytes(first.start, first.end) == 0
	})
	return candidates
}

// visitSegmentRanges enumerates every run range that respects the background
// boundaries, rolling the position-independent hash as the range extends.
func visitSegmentRanges(markup []rowMarkup, visitor func(row, start, end int, hash uint64)) {
	for rowIndex := range markup {
		m := &markup[rowIndex]
		if m.prefixLen[len(m.row.Runs)] == 0 {
			continue
		}
		for start := range m.row.Runs {
			if !m.boundary[start] {
				continue
			}
			h := uint64(segmentHashOffset)
			for end := start + 1; end <= len(m.row.Runs) && end-start <= maxSegmentRuns; end++ {
				if end-1 > start {
					h = (h ^ m.gapHash[end-1]) * segmentHashPrime
				}
				h = (h ^ m.runHash[end-1]) * segmentHashPrime
				if !m.boundary[end] {
					continue
				}
				visitor(rowIndex, start, end, h)
			}
		}
	}
}

func segmentRunsEqual(normalized, runs []ir.TextRun) bool {
	if len(normalized) != len(runs) {
		return false
	}
	base := runs[0].StartCol
	for i, run := range runs {
		if normalized[i].Text != run.Text || normalized[i].StartCol != run.StartCol-base ||
			normalized[i].EndCol != runEndCol(run)-base || normalized[i].Attrs != run.Attrs {
			return false
		}
	}
	return true
}

// selectSegments greedily accepts candidates by their estimated total saving.
// Every occurrence whose runs are still unclaimed becomes a user, so nested and
// overlapping candidates resolve deterministically in favour of the larger win.
func (c *canvas) selectSegments(
	markup []rowMarkup,
	candidates []*segmentCandidate,
	claimed [][]bool,
) []*segmentCandidate {
	const estimatedIDLen = 2
	estimate := func(candidate *segmentCandidate) int {
		c.chooseSegmentAnchor(markup, candidate, candidate.occurrences)
		return c.segmentSavings(markup, candidate, candidate.occurrences, estimatedIDLen)
	}
	estimates := make(map[*segmentCandidate]int, len(candidates))
	for _, candidate := range candidates {
		estimates[candidate] = estimate(candidate)
	}
	slices.SortFunc(candidates, func(a, b *segmentCandidate) int {
		if estimates[a] != estimates[b] {
			return estimates[b] - estimates[a]
		}
		return a.order - b.order
	})

	accepted := make([]*segmentCandidate, 0)
	for _, candidate := range candidates {
		if estimates[candidate] <= 0 {
			break
		}
		users := make([]segmentOccurrence, 0, len(candidate.occurrences))
		for _, occurrence := range candidate.occurrences {
			if segmentRunsFree(claimed[occurrence.row], occurrence.start, occurrence.end) {
				users = append(users, occurrence)
				markSegmentRuns(claimed[occurrence.row], occurrence.start, occurrence.end, true)
			}
		}
		release := func() {
			for _, user := range users {
				markSegmentRuns(claimed[user.row], user.start, user.end, false)
			}
		}
		if len(users) < 2 {
			release()
			continue
		}
		c.chooseSegmentAnchor(markup, candidate, users)
		idLen := len(compactXMLID(len(accepted)))
		if c.segmentSavings(markup, candidate, users, idLen) <= 0 {
			release()
			continue
		}
		candidate.users = users
		accepted = append(accepted, candidate)
	}
	return accepted
}

// finalizeSegmentIDs assigns the shortest identifiers to the most referenced
// definitions and drops any candidate whose exact identifier cost makes it
// unprofitable. Dropping only shortens later identifiers, so one pass per drop
// converges quickly.
func (c *canvas) finalizeSegmentIDs(
	markup []rowMarkup,
	accepted []*segmentCandidate,
	claimed [][]bool,
	ids *xmlIDAllocator,
) []*segmentCandidate {
	slices.SortStableFunc(accepted, func(a, b *segmentCandidate) int {
		return len(b.users) - len(a.users)
	})
	for {
		probe := *ids
		dropped := false
		for _, candidate := range accepted {
			id := probe.allocate()
			if c.segmentSavings(markup, candidate, candidate.users, len(id)) <= 0 {
				candidate.dropped = true
				dropped = true
				for _, user := range candidate.users {
					markSegmentRuns(claimed[user.row], user.start, user.end, false)
				}
				continue
			}
			candidate.def = &renderedRow{id: id}
		}
		accepted = slices.DeleteFunc(accepted, func(candidate *segmentCandidate) bool { return candidate.dropped })
		if !dropped {
			*ids = probe
			return accepted
		}
	}
}

func (c *canvas) chooseSegmentAnchor(markup []rowMarkup, candidate *segmentCandidate, users []segmentOccurrence) {
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
	candidate.anchorCol, candidate.anchorY = best.col, best.y
	for _, user := range users {
		if user.col == best.col && user.y == best.y {
			candidate.inlineBytes = markup[user.row].bytes(user.start, user.end)
			candidate.elements = markup[user.row].elements(user.start, user.end)
			break
		}
	}
}

func (c *canvas) segmentSavings(
	markup []rowMarkup,
	candidate *segmentCandidate,
	users []segmentOccurrence,
	idLen int,
) int {
	savings := -(candidate.inlineBytes + segmentDefinitionOverhead(idLen, candidate.elements))
	for _, user := range users {
		dx, dy := (user.col-candidate.anchorCol)*ColWidth, (user.y-candidate.anchorY)*RowHeight
		savings += markup[user.row].bytes(user.start, user.end) - c.segmentUseBytes(idLen, dx, dy)
	}
	return savings
}

func segmentRunsFree(claimed []bool, start, end int) bool {
	for k := start; k < end; k++ {
		if claimed[k] {
			return false
		}
	}
	return true
}

func markSegmentRuns(claimed []bool, start, end int, value bool) {
	for k := start; k < end; k++ {
		claimed[k] = value
	}
}

func (c *canvas) segmentDefinition(candidate *segmentCandidate) *renderedRow {
	row := ir.Row{Y: candidate.anchorY, Runs: make([]ir.TextRun, len(candidate.runs))}
	for i, run := range candidate.runs {
		row.Runs[i] = run
		row.Runs[i].StartCol += candidate.anchorCol
		row.Runs[i].EndCol += candidate.anchorCol
	}
	var sb strings.Builder
	c.writeRow(&sb, row)
	def := candidate.def
	def.row = row
	def.svg = sb.String()
	def.elements = candidate.elements
	def.count = len(candidate.users)
	def.definition = c.rowDefinition(def, def.id)
	return def
}
