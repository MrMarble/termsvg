package svg

import (
	"bytes"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// blockDefinitionPattern matches a definition whose body is made of
// references only, which is how a block of shared rows serializes.
var blockDefinitionPattern = regexp.MustCompile(
	`<g id="([a-z]+)">((?:<use href="#[a-z]+"(?: [xy]="-?[0-9]+")*/>)+)</g>`)

// boxRecording draws a three-row box that moves two columns to the right on
// every state. Every row recurs at a new column, so segment sharing turns each
// row into one offset reference, and the three references of a state recur as
// a group at a new column in every other state.
func boxRecording(states int) *ir.Recording {
	rows := make([][]ir.Row, states)
	for i := range states {
		col := 2 * i
		rows[i] = []ir.Row{
			parityRow(1, parityRun("+----------+", col, ir.CellAttrs{})),
			parityRow(2,
				parityRun("|", col, ir.CellAttrs{}),
				parityRun(" moving ", col+2, ir.CellAttrs{Bold: true}),
				parityRun("|", col+11, ir.CellAttrs{}),
			),
			parityRow(3, parityRun("+==========+", col, ir.CellAttrs{})),
		}
	}
	rec := parityRecording(60, 5, rows)
	for i := range rec.Frames {
		rec.Frames[i].Cursor = ir.Cursor{}
	}
	return rec
}

// pairRecording shows the same two rows at the top of the screen in two
// states and five rows lower in four states, with a counter that keeps every
// state distinct. The lower position is the more frequent one.
func pairRecording() *ir.Recording {
	alpha := func(y int) ir.Row {
		return parityRow(y, parityRun("alpha: a long row that moves as a pair", 0, ir.CellAttrs{}))
	}
	beta := func(y int) ir.Row {
		return parityRow(y, parityRun("beta:  a long row that moves as a pair", 0, ir.CellAttrs{}))
	}
	states := make([][]ir.Row, 6)
	for i := range states {
		y := 0
		if i >= 2 {
			y = 5
		}
		states[i] = []ir.Row{
			alpha(y), beta(y + 1), parityRow(7, parityRun("frame "+strconv.Itoa(i), 0, ir.CellAttrs{})),
		}
	}
	rec := parityRecording(60, 8, states)
	for i := range rec.Frames {
		rec.Frames[i].Cursor = ir.Cursor{}
	}
	return rec
}

func renderBlocksSVG(t *testing.T, rec *ir.Recording, opts ...Option) string {
	t.Helper()
	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig(), opts...).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

func withoutBlockSharing(options *Options) { options.withoutBlockSharing = true }

func TestBlockSharingSharesMovingBox(t *testing.T) {
	const states = 20
	rec := boxRecording(states)
	svg := renderBlocksSVG(t, rec)
	blocks := blockDefinitionPattern.FindAllStringSubmatch(svg, -1)
	if len(blocks) != 1 || strings.Count(blocks[0][2], "<use ") != 3 {
		t.Fatalf("block definitions = %d, want one with three references:\n%s", len(blocks), svg)
	}
	id := blocks[0][1]
	for i := range states {
		want := `<use href="#` + id + `"`
		if i > 0 {
			want += ` x="` + strconv.Itoa(i*2*ColWidth) + `"`
		}
		if !strings.Contains(svg, want+"/>") {
			t.Fatalf("missing block reference %q:\n%s", want+"/>", svg)
		}
	}
	if got := strings.Count(svg, `<use href="#`+id+`"`); got != states {
		t.Fatalf("block referenced %d times, want %d:\n%s", got, states, svg)
	}
	for _, once := range []string{"+----------+", ">moving<", "+==========+"} {
		if got := strings.Count(svg, once); got != 1 {
			t.Fatalf("%q serialized %d times, want once:\n%s", once, got, svg)
		}
	}
	plain := renderBlocksSVG(t, rec, withoutBlockSharing)
	if blockDefinitionPattern.MatchString(plain) {
		t.Fatalf("block sharing was not disabled:\n%s", plain)
	}
	if len(svg) >= len(plain) {
		t.Fatalf("block sharing grew the output from %d to %d bytes", len(plain), len(svg))
	}
}

func TestBlockSharingAnchorsAtMostFrequentPosition(t *testing.T) {
	rec := pairRecording()
	svg := renderBlocksSVG(t, rec)
	blocks := blockDefinitionPattern.FindAllStringSubmatch(svg, -1)
	if len(blocks) != 1 || strings.Count(blocks[0][2], "<use ") != 2 {
		t.Fatalf("block definitions = %d, want one with two references:\n%s", len(blocks), svg)
	}
	id := blocks[0][1]
	if got := strings.Count(svg, `<use href="#`+id+`"/>`); got != 4 {
		t.Fatalf("anchored references = %d, want the four lower states:\n%s", got, svg)
	}
	offset := `<use href="#` + id + `" y="` + strconv.Itoa(-5*RowHeight) + `"/>`
	if got := strings.Count(svg, offset); got != 2 {
		t.Fatalf("%q used %d times, want the two upper states:\n%s", offset, got, svg)
	}
	if strings.Count(svg, "alpha:") != 1 || strings.Count(svg, "beta:") != 1 {
		t.Fatalf("pair rows serialized more than once:\n%s", svg)
	}
	plain := renderBlocksSVG(t, rec, withoutBlockSharing)
	if len(svg) >= len(plain) {
		t.Fatalf("block sharing grew the output from %d to %d bytes", len(plain), len(svg))
	}
}

func TestBlockSharingPreservesSemanticsAcrossLayouts(t *testing.T) {
	for name, rec := range map[string]*ir.Recording{"box": boxRecording(6), "pair": pairRecording()} {
		for _, variant := range parityOptions {
			t.Run(name+"/"+variant.name, func(t *testing.T) { assertSemanticParity(t, rec, variant.options...) })
		}
		for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions, LayoutScroll} {
			t.Run(name+"/"+string(layout)+"/auto-style", func(t *testing.T) {
				assertSemanticParity(t, rec, WithLayout(layout), WithStyleMode(StyleAuto))
			})
		}
	}
}

func TestBlockSharingMetricsMatchSerializedStructure(t *testing.T) {
	for _, fixture := range []struct {
		name         string
		rec          *ir.Recording
		requireBlock bool
	}{{name: "box", rec: boxRecording(6), requireBlock: true}, {name: "pair", rec: pairRecording()}} {
		for _, variant := range parityOptions {
			// The region optimizer splits the moving box into narrow per-column
			// viewports, so no multi-row group recurs there; the pair fixture
			// keeps the regions accounting covered.
			if fixture.requireBlock && strings.HasPrefix(variant.name, "regions") {
				continue
			}
			for _, style := range []StyleMode{StyleLegacy, StyleAuto} {
				options := append(slices.Clone(variant.options), WithStyleMode(style))
				t.Run(fixture.name+"/"+variant.name+"/"+string(style), func(t *testing.T) {
					assertSharedBlockMetrics(t, fixture.rec, options, fixture.requireBlock)
				})
			}
		}
	}
}

func assertSharedBlockMetrics(t *testing.T, rec *ir.Recording, opts []Option, requireBlock bool) {
	t.Helper()
	config := renderer.DefaultConfig()
	options := DefaultOptions()
	for _, apply := range opts {
		apply(&options)
	}
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, options.MaxFPS, config.LoopCount)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := prepareCandidate(context.Background(), rec, &plan, config, &options)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	parsed := CandidateMetrics{}
	writer := &candidateWriter{w: &out, metrics: &parsed}
	if err := New(config, opts...).serializeCandidate(context.Background(), rec, writer, candidate); err != nil {
		t.Fatal(err)
	}
	writer.finish()
	shared := slices.ContainsFunc(candidate.content.rowDefs, func(def *renderedRow) bool { return len(def.items) > 0 })
	if requireBlock && !shared {
		t.Fatalf("block sharing did not apply:\n%s", out.String())
	}
	got := candidate.metrics
	if candidate.cost.finalBytes != parsed.FinalBytes {
		t.Fatalf("prepared bytes = %d, serialized %d", candidate.cost.finalBytes, parsed.FinalBytes)
	}
	if got.XMLNodes != parsed.XMLNodes || got.DefinitionNodes != parsed.DefinitionNodes ||
		got.ActiveNodes != parsed.ActiveNodes || got.TextNodes != parsed.TextNodes ||
		got.RectNodes != parsed.RectNodes || got.GroupNodes != parsed.GroupNodes ||
		got.UseNodes != parsed.UseNodes || got.AnimationNodes != parsed.AnimationNodes ||
		got.AnimatedElements != parsed.AnimatedElements {
		t.Fatalf("prepared metrics = %#v; serialized = %#v", got, parsed)
	}
	if requireBlock && (got.MaxUseDepth < 2 || got.StaticUseShadowNodes == 0 && got.PeakAnimatedUseShadowNodes == 0) {
		t.Fatalf("use expansion metrics ignore shared blocks: %#v", got)
	}
}

func TestBlockSharingRejectsUnprofitableGroups(t *testing.T) {
	// Two short rows recurring once at another position cannot pay for a
	// definition and an offset reference.
	rec := parityRecording(20, 4, [][]ir.Row{
		{parityRow(0, parityRun("x", 0, ir.CellAttrs{})), parityRow(1, parityRun("y", 0, ir.CellAttrs{}))},
		{parityRow(2, parityRun("x", 5, ir.CellAttrs{})), parityRow(3, parityRun("y", 5, ir.CellAttrs{}))},
	})
	svg := renderBlocksSVG(t, rec)
	if strings.Contains(svg, `<use href="#`) {
		t.Fatalf("an unprofitable row group was shared:\n%s", svg)
	}
	if plain := renderBlocksSVG(t, rec, withoutBlockSharing); plain != svg {
		t.Fatalf("output differs without block sharing:\n%s\n%s", svg, plain)
	}
}

func TestBlockDefinitionUsesExactMarkupBytes(t *testing.T) {
	const states = 8
	rec := boxRecording(states)
	c := &canvas{rec: rec, config: *renderer.DefaultConfig(), classNames: rec.Colors.GenerateClassNames()}
	c.plan = buildRenderPlan(rec, false)
	_, contentStates := c.contentKeyframes()
	frames, defs := c.collectRows(contentStates)
	block := defs[len(defs)-1]
	if len(block.items) != 3 || len(block.rows) != 3 || block.elements != 3 || len(block.uses) != 3 ||
		block.count != states || !rowEqual(block.row, block.rows[0]) {
		t.Fatalf("block definition = %#v", block)
	}
	var body strings.Builder
	for i, item := range block.items {
		if !rowEqual(item.row, block.rows[i]) || item.row.Y != i+1 {
			t.Fatalf("block item %d = %#v; want anchor row %d", i, item.row, i+1)
		}
		if item.id != "" {
			body.WriteString(c.segmentUseMarkup(item.id, 0, 0))
			continue
		}
		body.WriteString(item.svg)
	}
	if block.svg != body.String() || block.definition != `<g id="`+block.id+`">`+body.String()+`</g>` {
		t.Fatalf("block definition = %q; want the anchor rows %q", block.definition, body.String())
	}
	wrapper := segmentDefinitionOverhead(len(block.id), 3)
	if got := finalSVGBytes(block.definition, false); got != len(body.String())+wrapper {
		t.Fatalf("definition bytes = %d; want body plus wrapper", got)
	}
	if painted := block.paintRows(); len(painted) != 3 {
		t.Fatalf("block definition paints %d rows, want its three inline items", len(painted))
	}
	for i, rows := range frames {
		if len(rows) != 1 {
			t.Fatalf("state %d holds %d items, want one block reference", i, len(rows))
		}
		item := rows[0]
		want := c.segmentUseMarkup(block.id, i*2*ColWidth, 0)
		if item.svg != want || item.elements != 1 || !slices.Equal(item.uses, []string{block.id}) ||
			len(item.rows) != 3 || item.rows[0].Y != 1 || item.rows[2].Y != 3 || !rowEqual(item.row, item.rows[0]) {
			t.Fatalf("state %d block reference = %#v; want %q", i, item, want)
		}
		if len(item.paintRows()) != 0 || len(item.paintRow().Runs) != 0 {
			t.Fatalf("state %d block reference paints rows itself", i)
		}
		if c.stateNeedsWrapper(rows) || c.stateElementCount(rows) != 1 {
			t.Fatalf("state %d made of one block reference needs a wrapper", i)
		}
	}
}
