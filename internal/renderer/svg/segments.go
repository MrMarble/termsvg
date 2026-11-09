package svg

import (
	"slices"
	"strings"

	"github.com/mrmarble/termsvg/internal/ir"
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

// segmentCandidate shares occurrence units that are indexes into the unique
// row list and ranges that are run indexes.
type segmentCandidate struct {
	shareCandidate
	runs []ir.TextRun // normalized to column 0
	def  *renderedRow
}

// segmentUnits measures run ranges of the unique rows for the sharing engine.
type segmentUnits struct {
	markup  []rowMarkup
	claimed [][]bool
}

func (u segmentUnits) Bytes(unit, start, end int) int    { return u.markup[unit].bytes(start, end) }
func (u segmentUnits) Elements(unit, start, end int) int { return u.markup[unit].elements(start, end) }
func (u segmentUnits) Claimed(unit int) []bool           { return u.claimed[unit] }

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
)

func (rw rowWriter) buildRowMarkup(row ir.Row) rowMarkup {
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
	m.spans = rw.backgroundSpans(row)
	m.rects = make([]string, len(m.spans))
	for i := range m.boundary {
		m.boundary[i] = true
	}
	var sb strings.Builder
	for i, span := range m.spans {
		sb.Reset()
		rw.writeBackgroundSpan(&sb, row.Y, span)
		m.rects[i] = sb.String()
		m.byteAt[span.firstRun] += finalSVGBytes(m.rects[i], rw.minify)
		m.elemAt[span.firstRun]++
		// A merged background rectangle must not be split across a segment
		// boundary: it would change both paint order and rectangle count.
		for k := span.firstRun + 1; k <= span.lastRun; k++ {
			m.boundary[k] = false
		}
	}
	for k, run := range row.Runs {
		sb.Reset()
		rw.writeTextRun(&sb, run, row.Y)
		m.texts[k] = sb.String()
		if m.texts[k] != "" {
			m.byteAt[k] += finalSVGBytes(m.texts[k], rw.minify)
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
func (m *rowMarkup) render(minify bool, uses []rowSegmentUse) (svg string, elements int, inline ir.Row) {
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
	emitInline := func(until int) {
		for ; next < until; next++ {
			sb.WriteString(m.texts[next])
			inline.Runs = append(inline.Runs, m.row.Runs[next])
			if m.texts[next] != "" {
				elements++
			}
		}
	}
	for _, use := range uses {
		emitInline(use.start)
		sb.WriteString(segmentUseMarkup(minify, use.def.id, use.dx, use.dy))
		elements++
		next = use.end
	}
	emitInline(len(m.texts))
	return sb.String(), elements, inline
}

func segmentUseMarkup(minify bool, id string, dx, dy int) string {
	var sb strings.Builder
	sb.WriteString(`<use href="#`)
	sb.WriteString(id)
	sb.WriteByte('"')
	if dx != 0 {
		sb.WriteString(` x="`)
		sb.WriteString(formatInt(dx, minify))
		sb.WriteByte('"')
	}
	if dy != 0 {
		sb.WriteString(` y="`)
		sb.WriteString(formatInt(dy, minify))
		sb.WriteByte('"')
	}
	sb.WriteString(`/>`)
	return sb.String()
}

func segmentUseBytes(minify bool, idLen, dx, dy int) int {
	bytes := len(`<use href="#`) + idLen + len(`"/>`)
	if dx != 0 {
		bytes += len(` x=""`) + len(formatInt(dx, minify))
	}
	if dy != 0 {
		bytes += len(` y=""`) + len(formatInt(dy, minify))
	}
	return bytes
}

func normalizedSegmentRuns(runs []ir.TextRun) []ir.TextRun {
	base := runs[0].StartCol
	out := make([]ir.TextRun, len(runs))
	for i, run := range runs {
		out[i] = ir.TextRun{Text: run.Text, StartCol: run.StartCol - base, EndCol: runEndCol(run) - base, Attrs: run.Attrs}
	}
	return out
}

// segmentRunHash hashes a run independently of its absolute position. The
// distance to the preceding run is mixed in separately while extending a
// candidate, so equal run sequences hash equally wherever they occur.
func segmentRunHash(run ir.TextRun) uint64 {
	h := hashString(segmentHashOffset, run.Text)
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
func (rc rowCollector) shareRowSegments(markup []rowMarkup, ids *xmlIDAllocator) ([][]rowSegmentUse, []*renderedRow) {
	candidates := collectSegmentCandidates(markup)
	if len(candidates) == 0 {
		return make([][]rowSegmentUse, len(markup)), nil
	}
	claimed := make([][]bool, len(markup))
	for i := range markup {
		claimed[i] = make([]bool, len(markup[i].row.Runs))
	}
	units := segmentUnits{markup: markup, claimed: claimed}
	const estimatedIDLen = 2
	accepted := selectShared(rc.rows.minify, units, candidates, estimatedIDLen,
		func(accepted int) int { return len(compactXMLID(accepted)) })
	accepted = finalizeShared(rc.rows.minify, units, accepted, ids)

	uses := make([][]rowSegmentUse, len(markup))
	defs := make([]*renderedRow, 0, len(accepted))
	for _, candidate := range accepted {
		candidate.def = rc.segmentDefinition(candidate)
		defs = append(defs, candidate.def)
		for _, user := range candidate.users {
			uses[user.unit] = append(uses[user.unit], rowSegmentUse{
				start: user.start, end: user.end, def: candidate.def,
				dx: (user.col - candidate.anchor.col) * ColWidth, dy: (user.y - candidate.anchor.y) * RowHeight,
			})
		}
	}
	for i := range uses {
		slices.SortFunc(uses[i], func(a, b rowSegmentUse) int { return a.start - b.start })
	}
	return uses, defs
}

func collectSegmentCandidates(markup []rowMarkup) []*segmentCandidate {
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
			candidate = &segmentCandidate{
				shareCandidate: shareCandidate{order: len(candidates)}, runs: normalizedSegmentRuns(runs),
			}
			byHash[hash] = append(byHash[hash], candidate)
			candidates = append(candidates, candidate)
		}
		candidate.occurrences = append(candidate.occurrences, occurrence{
			unit: row, start: start, end: end, col: runs[0].StartCol, y: markup[row].row.Y,
		})
	})
	candidates = slices.DeleteFunc(candidates, func(candidate *segmentCandidate) bool {
		first := candidate.occurrences[0]
		return len(candidate.occurrences) < 2 || markup[first.unit].bytes(first.start, first.end) == 0
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

func (rc rowCollector) segmentDefinition(candidate *segmentCandidate) *renderedRow {
	row := ir.Row{Y: candidate.anchor.y, Runs: make([]ir.TextRun, len(candidate.runs))}
	for i, run := range candidate.runs {
		row.Runs[i] = run
		row.Runs[i].StartCol += candidate.anchor.col
		row.Runs[i].EndCol += candidate.anchor.col
	}
	var sb strings.Builder
	rc.rows.writeRow(&sb, row)
	def := &renderedRow{id: candidate.id}
	def.row = row
	def.svg = sb.String()
	def.elements = candidate.elements
	def.count = len(candidate.users)
	def.definition = rc.rows.rowDefinition(def, def.id)
	return def
}
