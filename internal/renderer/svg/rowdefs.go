package svg

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mrmarble/termsvg/internal/ir"
)

type renderedRow struct {
	row        ir.Row
	svg        string
	definition string
	count      int
	order      int
	id         string
	// elements is the number of XML elements in svg. It is zero for rows
	// constructed without segment sharing, where the row itself is authoritative.
	elements int
	// uses lists the shared definitions referenced by svg, in order: segment
	// identifiers, and for a block the identifiers its covered rows reference.
	uses []string
	// inline holds the runs serialized directly when uses is not empty.
	inline ir.Row
	// rows holds the rows a shared block covers: the anchor rows of a block
	// definition, or the rows a block reference replaces. row is rows[0].
	rows []ir.Row
	// items holds the rendered rows a block definition's body is made of. It
	// is nil for a block reference, whose paint is drawn through the definition.
	items []*renderedRow
}

// xmlIDAllocator hands out compact XML identifiers in order, skipping the
// reserved clip path identifier.
// rowCollector deduplicates rendered rows into shared definitions: whole rows,
// recurring segments and multi-row blocks. It needs only the row writer and
// whether block sharing is disabled.
type rowCollector struct {
	rows                rowWriter
	withoutBlockSharing bool
}

type xmlIDAllocator struct{ next int }

// paintRow returns the runs whose paint is serialized by this row itself,
// excluding runs that are drawn through shared references. Block entries
// paint several rows or none; paintRows lists them.
func (r *renderedRow) paintRow() ir.Row {
	if len(r.uses) > 0 || len(r.rows) > 0 {
		return r.inline
	}
	return r.row
}

// paintRows returns every row whose paint this entry serializes itself: the
// inline items of a block definition, nothing for a block reference, and the
// paint row of any other entry.
func (r *renderedRow) paintRows() []ir.Row {
	if len(r.rows) == 0 {
		return []ir.Row{r.paintRow()}
	}
	rows := make([]ir.Row, 0, len(r.items))
	for _, item := range r.items {
		if item.id == "" {
			rows = append(rows, item.paintRow())
		}
	}
	return rows
}

func (a *xmlIDAllocator) allocate() string {
	for {
		id := compactXMLIDAt(a.next)
		a.next++
		if id != "clip" {
			return id
		}
	}
}

// compactXMLIDs allocates count identifiers starting at allocator index start
// and returns the index that follows them.
func compactXMLIDs(start, count int) (ids []string, next int) {
	allocator := xmlIDAllocator{next: start}
	ids = make([]string, count)
	for i := range ids {
		ids[i] = allocator.allocate()
	}
	return ids, allocator.next
}

// xmlIDIndexAfter returns the allocator index that follows count identifiers
// allocated from the start of the alphabet.
func xmlIDIndexAfter(count int) int {
	_, next := compactXMLIDs(0, count)
	return next
}

func (rc rowCollector) collectRows(contentStates [][]ir.Row) (frames [][]*renderedRow, defs []*renderedRow) {
	return rc.collectRowsWithHash(contentStates, semanticRowHash)
}

//nolint:funlen // each sharing stage measures the markup chosen by the one before
func (rc rowCollector) collectRowsWithHash(
	contentStates [][]ir.Row,
	hash func(ir.Row) uint64,
) (frames [][]*renderedRow, defs []*renderedRow) {
	seen := make(map[uint64][]*renderedRow)
	frames = make([][]*renderedRow, len(contentStates))
	ordered := make([]*renderedRow, 0)

	for i, rows := range contentStates {
		for _, row := range rows {
			key := hash(row)
			var entry *renderedRow
			for _, candidate := range seen[key] {
				if rowEqual(candidate.row, row) {
					entry = candidate
					break
				}
			}
			if entry == nil {
				entry = &renderedRow{row: row, order: len(ordered)}
				seen[key] = append(seen[key], entry)
				ordered = append(ordered, entry)
			}
			entry.count++
			frames[i] = append(frames[i], entry)
		}
	}

	// Shared segments are chosen first so whole-row interning below measures
	// the markup that will actually be serialized.
	markup := make([]rowMarkup, len(ordered))
	for i, entry := range ordered {
		markup[i] = rc.rows.buildRowMarkup(entry.row)
	}
	ids := &xmlIDAllocator{}
	uses, defs := rc.shareRowSegments(markup, ids)
	for i, entry := range ordered {
		entry.svg, entry.elements, entry.inline = markup[i].render(rc.rows.minify, uses[i])
		for _, use := range uses[i] {
			entry.uses = append(entry.uses, use.def.id)
		}
	}

	// Give the most frequently referenced rows the shortest identifiers. The
	// first-occurrence order is the deterministic tie-breaker.
	candidates := append([]*renderedRow(nil), ordered...)
	slices.SortStableFunc(candidates, func(a, b *renderedRow) int {
		if a.count != b.count {
			return b.count - a.count
		}
		return a.order - b.order
	})

	for _, entry := range candidates {
		if entry.count < 2 || entry.svg == "" {
			continue
		}
		probe := *ids
		id := probe.allocate()
		definition := rc.rows.rowDefinition(entry, id)
		use := useMarkup(rc.rows.minify, id, 0, 0)
		inlineBytes := entry.count * finalSVGBytes(entry.svg, rc.rows.minify)
		definitionBytes := finalSVGBytes(definition, rc.rows.minify) +
			entry.count*finalSVGBytes(use, rc.rows.minify)
		if definitionBytes < inlineBytes {
			*ids = probe
			entry.id = id
			entry.definition = definition
			defs = append(defs, entry)
		}
	}

	// Recurring groups of consecutive rows are shared last, measured against
	// the references and markup chosen above.
	if !rc.withoutBlockSharing {
		frames, defs = rc.shareRowBlocks(frames, defs, ids)
	}
	return frames, defs
}

func compactXMLIDAt(index int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	index++
	var id [16]byte
	pos := len(id)
	for index > 0 {
		index--
		pos--
		id[pos] = alphabet[index%len(alphabet)]
		index /= len(alphabet)
	}
	return string(id[pos:])
}

func finalSVGBytes(value string, minified bool) int {
	bytes := len(value)
	if minified {
		// svgoutput converts every UTF-8 NBSP (two bytes) back to a
		// one-byte ASCII space after minification. Profitability must therefore
		// use the bytes that will actually reach the output file.
		bytes -= strings.Count(value, "\u00a0")
	}
	return bytes
}

func addElementID(svg, id string) string {
	prefix := elementIDPrefix(svg)
	if prefix == "" {
		return `<g id="` + id + `">` + svg + `</g>`
	}
	return prefix + ` id="` + id + `"` + strings.TrimPrefix(svg, prefix)
}

func elementIDPrefix(svg string) string {
	end := strings.IndexByte(svg, '>')
	if end < 0 {
		return ""
	}
	start := svg[:end+1]
	for _, name := range []string{"rect", "text", "use"} {
		prefix := "<" + name
		if strings.HasPrefix(start, prefix) &&
			(len(start) == len(prefix)+1 || start[len(prefix)] == ' ' || start[len(prefix)] == '/') {
			if strings.Contains(start, ` id="`) {
				return ""
			}
			return prefix
		}
	}
	return ""
}

func semanticRowHash(row ir.Row) uint64 {
	h := uint64(hashOffset)
	h = mixHash(h, row.Y)
	h = mixHash(h, len(row.Runs))
	for _, run := range row.Runs {
		h = mixHash(h, len(run.Text))
		h = hashString(h, run.Text)
		h = mixHash(h, run.StartCol)
		h = mixHash(h, run.EndCol)
		h = mixHash(h, int(run.Attrs.FG))
		h = mixHash(h, int(run.Attrs.BG))
		h = mixHash(h, attrBits(run.Attrs))
	}
	return h
}

func (c *canvas) writeRowDefs(defs []*renderedRow) {
	for _, row := range defs {
		fmt.Fprint(c.w, row.definition)
	}
}

func (c *canvas) writeStateDefs(content *preparedContent) {
	if c.options.FrameSwitch != FrameSwitchHref {
		return
	}
	for i, id := range content.frameStateIDs {
		if ownsStateDefinition(content.frameStateIDs, content.frameRows, i) {
			c.writeStateDefinition(id, content.frameRows[i])
		}
	}
	for bandIndex := range content.bands {
		band := &content.bands[bandIndex]
		for i, id := range band.stateIDs {
			if ownsStateDefinition(band.stateIDs, band.rows, i) {
				c.writeStateDefinition(id, band.rows[i])
			}
		}
	}
}

func (c *canvas) writeStateDefinition(id string, rows []*renderedRow) {
	if !c.rows().stateNeedsWrapper(rows) {
		var body strings.Builder
		writer := c.w
		c.w = &body
		c.writeFrameRows(rows)
		c.w = writer
		fmt.Fprint(c.w, addElementID(body.String(), id))
		return
	}
	if c.rows().stateElementCount(rows) == 0 && c.config.Minify {
		fmt.Fprintf(c.w, `<g id="%s"/>`, id)
		return
	}
	fmt.Fprintf(c.w, `<g id="%s">`, id)
	c.writeFrameRows(rows)
	fmt.Fprint(c.w, `</g>`)
}
