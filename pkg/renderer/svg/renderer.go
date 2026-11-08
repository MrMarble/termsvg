// Package svg provides an SVG renderer for terminal recordings.
// It generates animated SVGs using CSS keyframes or discrete SMIL timelines.
package svg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mrmarble/termsvg/internal/svgoutput"
	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// Renderer implements the renderer.Renderer interface for SVG output.
type Renderer struct {
	config               renderer.Config
	options              Options
	onSemanticPlanBuild  func()
	onCandidateSerialize func()
}

// canvas holds rendering state
type canvas struct {
	w          io.Writer
	rec        *ir.Recording
	plan       renderPlan
	config     renderer.Config
	options    Options
	classNames map[color.ID]string
	style      stylePlan
	metrics    *CandidateMetrics
}

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

type backgroundSpan struct {
	startCol int
	endCol   int
	colorID  color.ID
	firstRun int
	lastRun  int
}

// xmlIDAllocator hands out compact XML identifiers in order, skipping the
// reserved clip path identifier.
type xmlIDAllocator struct{ next int }

type preparedCandidate struct {
	plan    *semanticPlan
	options Options
	content preparedContent
	metrics CandidateMetrics
	cost    preparedCandidateCost
	style   stylePlan
}

type countingWriter struct {
	bytes        int64
	collapseNBSP bool
	pendingC2    bool
}

// Layout constants for SVG rendering
const (
	RowHeight  = 25 // pixels per row
	ColWidth   = 12 // pixels per column
	Padding    = 20 // padding around content
	HeaderSize = 2  // multiplier for header area (window buttons)

	// windowCornerRadius is the radius for rounded window corners.
	windowCornerRadius = 5

	// windowButtonSpacing is the horizontal spacing between window buttons.
	windowButtonSpacing = 20

	// windowButtonRadius is the radius of window control buttons.
	windowButtonRadius = 6
)

var svgTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

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

func (r *Renderer) buildSemanticPlan(ctx context.Context, rec *ir.Recording) (semanticPlan, error) {
	if r.onSemanticPlanBuild != nil {
		r.onSemanticPlanBuild()
	}
	return buildSemanticPlan(ctx, rec, r.config.ShowCursor, r.options.MaxFPS, r.config.LoopCount)
}

// New creates a new SVG renderer with the given configuration.
func New(config *renderer.Config, opts ...Option) *Renderer {
	options := DefaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return &Renderer{config: *config, options: options.normalized()}
}

// Format returns the output format name.
func (r *Renderer) Format() string {
	return "svg"
}

// FileExtension returns the file extension for SVG files.
func (r *Renderer) FileExtension() string {
	return ".svg"
}

// Render generates an animated SVG from the recording.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	if len(rec.Frames) == 0 {
		return fmt.Errorf("recording has no frames")
	}
	if err := r.options.Validate(); err != nil {
		return err
	}
	candidate, err := r.prepareBestCandidate(ctx, rec)
	if err != nil {
		return err
	}
	return r.serializeCandidate(ctx, rec, w, candidate)
}

// prepareBestCandidate prepares the configured candidate for every plan
// variant and keeps the one the objective prefers. Interval-static layers are
// chosen by an estimate, so the plan without them is measured as well.
func (r *Renderer) prepareBestCandidate(ctx context.Context, rec *ir.Recording) (*preparedCandidate, error) {
	plan, err := r.buildSemanticPlan(ctx, rec)
	if err != nil {
		return nil, err
	}
	layered, err := r.selectIntervalLayers(ctx, rec, &plan)
	if err != nil {
		return nil, err
	}
	plans := []semanticPlan{plan}
	if len(layered.layers) > 0 {
		plans = append(plans, layered)
	}
	candidates := make([]*preparedCandidate, 0, len(plans))
	for i := range plans {
		candidate, err := r.prepareSelectedCandidate(ctx, rec, &plans[i])
		if err != nil {
			return nil, err
		}
		if candidate.metrics.FinalBytes == 0 {
			if err := r.measureCandidate(ctx, rec, candidate); err != nil {
				return nil, err
			}
		}
		candidates = append(candidates, candidate)
	}
	selected := selectPreparedCandidate(r.options.AutoObjective, candidates...)
	if r.config.Debug && len(candidates) > 1 {
		for _, candidate := range candidates {
			log.Printf("[SVG] plan variant layers=%d bytes=%d selected=%t",
				len(candidate.plan.layers), candidate.metrics.FinalBytes, candidate == selected)
		}
	}
	return selected, nil
}

// MeasureCandidate renders the configured candidate to a sink and reports its
// exact serialized size and structural cost.
func (r *Renderer) MeasureCandidate(ctx context.Context, rec *ir.Recording) (CandidateMetrics, error) {
	if len(rec.Frames) == 0 {
		return CandidateMetrics{}, fmt.Errorf("recording has no frames")
	}
	if err := r.options.Validate(); err != nil {
		return CandidateMetrics{}, err
	}
	candidate, err := r.prepareBestCandidate(ctx, rec)
	if err != nil {
		return CandidateMetrics{}, err
	}
	return candidate.metrics, nil
}

func (r *Renderer) prepareSelectedCandidate(
	ctx context.Context,
	rec *ir.Recording,
	plan *semanticPlan,
) (*preparedCandidate, error) {
	if r.options.Layout != LayoutAuto {
		return prepareCandidate(ctx, rec, plan, &r.config, &r.options)
	}
	options := r.options
	options.Layout = LayoutFrames
	frames, err := prepareCandidate(ctx, rec, plan, &r.config, &options)
	if err != nil {
		return nil, err
	}
	if err := r.measureCandidate(ctx, rec, frames); err != nil {
		return nil, err
	}
	options.Layout = LayoutBands
	bands, err := prepareCandidate(ctx, rec, plan, &r.config, &options)
	if err != nil {
		return nil, err
	}
	if err := r.measureCandidate(ctx, rec, bands); err != nil {
		return nil, err
	}
	options.Layout = LayoutRegions
	regions, err := prepareCandidate(ctx, rec, plan, &r.config, &options)
	if err != nil {
		return nil, err
	}
	if err := r.measureCandidate(ctx, rec, regions); err != nil {
		return nil, err
	}
	selected := selectPreparedCandidate(r.options.AutoObjective, frames, bands, regions)
	r.logAutoCandidates(selected, frames, bands, regions)
	return selected, nil
}

func selectPreparedCandidate(objective AutoObjective, candidates ...*preparedCandidate) *preparedCandidate {
	selected := candidates[0]
	for _, candidate := range candidates[1:] {
		if objective == AutoObjectiveRuntime && runtimeCandidateLess(&candidate.metrics, &selected.metrics) ||
			objective != AutoObjectiveRuntime && candidate.metrics.FinalBytes < selected.metrics.FinalBytes {
			selected = candidate
		}
	}
	return selected
}

func runtimeCandidateLess(left, right *CandidateMetrics) bool {
	comparisons := [...]struct{ left, right uint64 }{
		{left.PeakLiveNodeEstimate, right.PeakLiveNodeEstimate},
		{left.DurationWeightedInstantiatedNodeNanos, right.DurationWeightedInstantiatedNodeNanos},
		{left.UseTargetSwitches, right.UseTargetSwitches},
	}
	for _, comparison := range comparisons {
		if comparison.left != comparison.right {
			return comparison.left < comparison.right
		}
	}
	integerComparisons := [...]struct{ left, right int64 }{
		{int64(left.AnimationNodes), int64(right.AnimationNodes)},
		{int64(left.AnimatedElements), int64(right.AnimatedElements)},
		{int64(left.LocalViewportCount), int64(right.LocalViewportCount)},
		{left.MaxViewportArea, right.MaxViewportArea},
		{left.MaxTranslatedArea, right.MaxTranslatedArea},
		{left.FinalBytes, right.FinalBytes},
	}
	for _, comparison := range integerComparisons {
		if comparison.left != comparison.right {
			return comparison.left < comparison.right
		}
	}
	return false
}

func (r *Renderer) logAutoCandidates(selected *preparedCandidate, candidates ...*preparedCandidate) {
	if !r.config.Debug {
		return
	}
	for _, candidate := range candidates {
		metrics := candidate.metrics
		log.Printf("[SVG] auto candidate layout=%s bytes=%d source_nodes=%d definition_nodes=%d "+
			"peak_instantiated_nodes=%d animation_nodes=%d viewport_count=%d viewport_area=%d translated_area=%d "+
			"objective=%s selected=%s",
			candidate.options.Layout, metrics.FinalBytes, metrics.SourceActiveNodes, metrics.SourceDefinitionNodes,
			metrics.PeakInstantiatedNodes, metrics.AnimationNodes, metrics.LocalViewportCount, metrics.MaxViewportArea,
			metrics.MaxTranslatedArea,
			r.options.AutoObjective, selected.options.Layout)
	}
}

func prepareCandidate(
	ctx context.Context,
	rec *ir.Recording,
	plan *semanticPlan,
	config *renderer.Config,
	options *Options,
) (*preparedCandidate, error) {
	if options.Style == StyleAuto {
		legacy, _, err := prepareStyleCandidate(ctx, rec, plan, config, options, styleLegacy)
		if err != nil {
			return nil, err
		}
		selected := legacy
		for _, scheme := range []styleScheme{styleAtomic, styleComposite, styleInheritedAtomic, styleInheritedComposite} {
			candidate, stable, err := prepareStyleCandidate(ctx, rec, plan, config, options, scheme)
			if err != nil {
				return nil, err
			}
			if stable && candidate.cost.finalBytes < selected.cost.finalBytes {
				selected = candidate
			}
		}
		attachStyleCostLedger(rec, plan, config, selected)
		return selected, nil
	}
	candidate, _, err := prepareStyleCandidate(ctx, rec, plan, config, options, styleLegacy)
	return candidate, err
}

func prepareStyleCandidate(
	ctx context.Context,
	rec *ir.Recording,
	plan *semanticPlan,
	config *renderer.Config,
	options *Options,
	scheme styleScheme,
) (*preparedCandidate, bool, error) {
	c := &canvas{
		rec:        rec,
		plan:       *plan,
		config:     *config,
		options:    *options,
		classNames: rec.Colors.GenerateClassNames(),
	}
	c.style = stylePlan{scheme: styleLegacy}
	if options.Style == StyleAuto {
		c.style = c.legacyStylePlan()
	}
	if scheme == styleLegacy {
		content, err := c.prepareContentContext(ctx)
		if err != nil {
			return nil, false, err
		}
		if options.Style == StyleAuto {
			c.style.occurrences = c.countPaintOccurrences(&content)
		}
		candidate, err := finishPreparedCandidate(c, plan, options, &content)
		return candidate, true, err
	}

	style := c.buildStylePlan(scheme, paintOccurrences{texts: map[textStyleKey]int{}, backgrounds: map[color.ID]int{}})
	// Bound the naming/interning fixed-point search to four passes. Schemes that
	// do not stabilize are skipped so another stable candidate can win.
	for range 4 {
		c.style = style
		content, err := c.prepareContentContext(ctx)
		if err != nil {
			return nil, false, err
		}
		next := c.buildStylePlan(scheme, c.countPaintOccurrences(&content))
		if stylePlanEqual(&style, &next) {
			c.style = next
			candidate, err := finishPreparedCandidate(c, plan, options, &content)
			return candidate, true, err
		}
		style = next
	}
	return nil, false, nil
}

func finishPreparedCandidate(
	c *canvas,
	plan *semanticPlan,
	options *Options,
	content *preparedContent,
) (*preparedCandidate, error) {
	candidate := &preparedCandidate{plan: plan, options: *options, content: *content, style: c.style}
	addPreparedMetrics(&candidate.metrics, content, options, c.contentWidth(), c.contentHeight())
	if err := addStructuralMetrics(&candidate.metrics, c, content); err != nil {
		return nil, err
	}
	candidate.cost = buildPreparedCandidateCost(c, candidate)
	return candidate, nil
}

func attachStyleCostLedger(
	rec *ir.Recording,
	plan *semanticPlan,
	config *renderer.Config,
	candidate *preparedCandidate,
) {
	c := &canvas{
		rec: rec, plan: *plan, config: *config, options: candidate.options,
		classNames: rec.Colors.GenerateClassNames(), style: candidate.style,
	}
	candidate.style.cost = c.buildStyleCostLedger()
	candidate.style.styleBytes = candidate.style.cost.total
}

func (r *Renderer) measureCandidate(ctx context.Context, rec *ir.Recording, candidate *preparedCandidate) error {
	bytes, err := costPreparedCandidate(ctx, rec, &r.config, candidate)
	if err != nil {
		return err
	}
	candidate.metrics.FinalBytes = bytes
	return nil
}

func writeFinalSVG(w io.Writer, minify bool, render func(io.Writer) error) error {
	if minify {
		return svgoutput.Write(w, render)
	}
	return render(w)
}

func (r *Renderer) serializeCandidate(
	ctx context.Context,
	rec *ir.Recording,
	w io.Writer,
	candidate *preparedCandidate,
) error {
	if r.onCandidateSerialize != nil {
		r.onCandidateSerialize()
	}
	buf := bufio.NewWriterSize(w, 64*1024)
	c := &canvas{
		w: buf, rec: rec, plan: *candidate.plan, config: r.config, options: candidate.options,
		classNames: rec.Colors.GenerateClassNames(), metrics: &candidate.metrics,
		style: candidate.style,
	}
	if err := c.render(ctx, &candidate.content); err != nil {
		return err
	}
	return buf.Flush()
}

func (w *countingWriter) Write(p []byte) (int, error) {
	if !w.collapseNBSP {
		w.bytes += int64(len(p))
		return len(p), nil
	}
	for _, value := range p {
		if w.pendingC2 {
			if value == 0xa0 {
				w.bytes++
				w.pendingC2 = false
				continue
			}
			w.bytes++
			w.pendingC2 = false
		}
		if value == 0xc2 {
			w.pendingC2 = true
		} else {
			w.bytes++
		}
	}
	return len(p), nil
}

func (w *countingWriter) size() int64 {
	if w.pendingC2 {
		return w.bytes + 1
	}
	return w.bytes
}

func (c *canvas) contentWidth() int {
	if c.plan.width == 0 {
		return c.rec.Width * ColWidth
	}
	return c.plan.width * ColWidth
}

func (c *canvas) contentHeight() int {
	if c.plan.height == 0 {
		return c.rec.Height * RowHeight
	}
	return c.plan.height * RowHeight
}

func (c *canvas) paddedWidth() int {
	return c.contentWidth() + 2*Padding
}

func (c *canvas) paddedHeight() int {
	if c.config.ShowWindow {
		return c.contentHeight() + Padding*HeaderSize + Padding
	}
	return c.contentHeight() + 2*Padding
}

func (c *canvas) render(ctx context.Context, content *preparedContent) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.writeSVGOpen()

	if c.config.ShowWindow {
		c.writeWindow()
	} else {
		c.writeBackground()
	}

	// Content group with clipping
	contentY := Padding
	if c.config.ShowWindow {
		contentY = Padding * HeaderSize
	}

	c.writeDefs(content)
	c.writeContentGroupOpen(contentY)

	c.writeStyles(content)
	for _, row := range c.plan.staticRows {
		c.writeRow(c.w, row)
	}
	c.writeLayers(c.w)
	c.writeContent(content)
	c.writeCursor()

	fmt.Fprint(c.w, `</g></svg>`)

	return nil
}

func (c *canvas) writeSVGOpen() {
	xmlSpace := ` xml:space="preserve"`
	if c.config.Minify {
		xmlSpace = ""
	}
	fmt.Fprintf(c.w, `<svg xmlns="http://www.w3.org/2000/svg"%s width="%s" height="%s">`,
		xmlSpace, c.xmlInt(c.paddedWidth()), c.xmlInt(c.paddedHeight()))
}

func (c *canvas) writeDefs(content *preparedContent) {
	fmt.Fprintf(c.w, `<defs><clipPath id="clip"><rect width="%s" height="%s"/></clipPath>`,
		c.xmlInt(c.contentWidth()), c.xmlInt(c.contentHeight()))
	c.writeRowDefs(content.rowDefs)
	c.writeStateDefs(content)
	fmt.Fprint(c.w, `</defs>`)
}

func (c *canvas) writeContentGroupOpen(contentY int) {
	fmt.Fprintf(c.w, `<g transform="translate(%s,%s)" clip-path="url(#clip)"%s>`,
		c.xmlInt(Padding), c.xmlInt(contentY), c.style.contentGroupAttributes)
}

func (c *canvas) writeBackground() {
	bgHex := color.RGBAtoHex(c.config.Theme.WindowBackground)
	fmt.Fprintf(c.w, `<rect width="100%%" height="100%%" fill="%s"/>`, bgHex)
}

func (c *canvas) writeWindow() {
	theme := c.config.Theme

	// Window background with rounded corners
	bgHex := color.RGBAtoHex(theme.WindowBackground)
	fmt.Fprintf(c.w, `<rect rx="%s" width="100%%" height="100%%" fill="%s"/>`, c.xmlInt(windowCornerRadius), bgHex)

	// Window buttons (close, minimize, maximize)
	buttonY := Padding
	for i, btnColor := range theme.WindowButtons {
		btnHex := color.RGBAtoHex(btnColor)
		x := Padding + i*windowButtonSpacing
		fmt.Fprintf(c.w, `<circle cx="%s" cy="%s" r="%s" fill="%s"/>`,
			c.xmlInt(x), c.xmlInt(buttonY), c.xmlInt(windowButtonRadius), btnHex)
	}
}

func (c *canvas) writeStyles(content *preparedContent) {
	var sb strings.Builder
	sb.WriteString("<style>")

	if c.options.Animation == AnimationCSS {
		if c.options.usesLocalViewports() {
			written := make(map[string]bool)
			for i := range content.bands {
				band := &content.bands[i]
				if band.name == "" || written[band.name] {
					continue
				}
				if band.kind == bandScrollTape {
					sb.WriteString(c.generateScrollKeyframes(band.name, band.offsets))
				} else {
					sb.WriteString(c.generateBandKeyframes(band.name, band.keyframes, band.width*ColWidth))
				}
				written[band.name] = true
			}
		} else if len(content.frameKeyframes) > 1 {
			sb.WriteString(c.generateKeyframes(content.frameKeyframes))
		}
		if len(c.cursorKeyframes()) > 1 {
			sb.WriteString(c.generateCursorKeyframes())
		}
		for index := range c.plan.layers {
			sb.WriteString(c.generateLayerKeyframes(index))
		}
	}
	if c.plan.cursorEverVisible {
		sb.WriteString("@keyframes blink{0%,50%{opacity:1}50.01%,100%{opacity:0}}")
	}

	c.writePaintStyles(&sb)

	sb.WriteString("</style>")
	fmt.Fprint(c.w, sb.String())
}

func (c *canvas) writePaintStyles(sb *strings.Builder) {
	// Default text style (white-space:pre preserves spaces, survives minification)
	fgHex := color.RGBAtoHex(c.rec.Colors.DefaultForeground())
	if c.style.scheme == "" || c.style.scheme == styleLegacy {
		fmt.Fprintf(sb, "text{font-family:%s;font-size:%dpx;fill:%s;white-space:pre}",
			c.config.FontFamily, c.config.FontSize, fgHex)
	} else {
		fmt.Fprintf(sb, "text{%s}", c.style.textBaseRule)
	}

	if c.plan.cursorEverVisible && (c.style.scheme == "" || c.style.scheme == styleLegacy) {
		fmt.Fprintf(sb, ".cursor{fill:%s;animation:blink 1s step-end infinite}", fgHex)
	}

	if c.style.scheme == "" || c.style.scheme == styleLegacy {
		// Color classes
		for _, id := range c.visibleColorIDs() {
			rgba := c.rec.Colors.Resolved(id)
			className := c.classNames[id]
			hex := color.RGBAtoHex(rgba)
			fmt.Fprintf(sb, ".%s{fill:%s}", className, hex)
		}

		// Attribute classes (only if used)
		if c.rec.Stats.HasBold {
			sb.WriteString(".bold{font-weight:bold}")
		}
		if c.rec.Stats.HasItalic {
			sb.WriteString(".italic{font-style:italic}")
		}
		if c.rec.Stats.HasUnderline {
			sb.WriteString(".underline{text-decoration:underline}")
		}
		if c.rec.Stats.HasDim {
			sb.WriteString(".dim{opacity:0.5}")
		}
	} else {
		for _, rule := range c.style.rules {
			fmt.Fprintf(sb, ".%s{%s}", rule.class, rule.declarations)
		}
	}
}

func (c *canvas) visibleColorIDs() []color.ID {
	return c.plan.usedColors
}

func (c *canvas) generateKeyframes(frames []keyframePoint[int]) string {
	var sb strings.Builder
	sb.WriteString("@keyframes k{")
	width := c.contentWidth()
	for _, frame := range frames {
		fmt.Fprintf(&sb, "%s{transform:translateX(%dpx)}", frame.selector, -width*frame.state)
	}

	sb.WriteString("}")
	return sb.String()
}

func (c *canvas) contentKeyframes() ([]keyframePoint[int], [][]ir.Row) {
	return contentKeyframesFor(c.plan.content)
}

func (c *canvas) collectRows(contentStates [][]ir.Row) (frames [][]*renderedRow, defs []*renderedRow) {
	return c.collectRowsWithHash(contentStates, semanticRowHash)
}

//nolint:funlen // each sharing stage measures the markup chosen by the one before
func (c *canvas) collectRowsWithHash(
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
		markup[i] = c.buildRowMarkup(entry.row)
	}
	ids := &xmlIDAllocator{}
	uses, defs := c.shareRowSegments(markup, ids)
	for i, entry := range ordered {
		entry.svg, entry.elements, entry.inline = markup[i].render(c, uses[i])
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
		definition := c.rowDefinition(entry, id)
		use := `<use href="#` + id + `"/>`
		inlineBytes := entry.count * finalSVGBytes(entry.svg, c.config.Minify)
		definitionBytes := finalSVGBytes(definition, c.config.Minify) +
			entry.count*finalSVGBytes(use, c.config.Minify)
		if definitionBytes < inlineBytes {
			*ids = probe
			entry.id = id
			entry.definition = definition
			defs = append(defs, entry)
		}
	}

	// Recurring groups of consecutive rows are shared last, measured against
	// the references and markup chosen above.
	if !c.options.withoutBlockSharing {
		frames, defs = c.shareRowBlocks(frames, defs, ids)
	}
	return frames, defs
}

func compactXMLID(index int) string {
	for {
		value := compactXMLIDAt(index)
		if value != "clip" {
			return value
		}
		index++
	}
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
		// The export pipeline converts every UTF-8 NBSP (two bytes) back to a
		// one-byte ASCII space after minification. Profitability must therefore
		// use the bytes that will actually reach the output file.
		bytes -= strings.Count(value, "\u00a0")
	}
	return bytes
}

func (c *canvas) rowDefinition(row *renderedRow, id string) string {
	if c.renderedElementCount(row) == 1 {
		return addElementID(row.svg, id)
	}
	return `<g id="` + id + `">` + row.svg + `</g>`
}

// renderedElementCount reports the XML elements a rendered row serializes.
// Rows produced by collectRows carry an exact count; rows constructed directly
// from IR fall back to counting their runs.
func (c *canvas) renderedElementCount(row *renderedRow) int {
	if row.elements > 0 {
		return row.elements
	}
	return c.rowElementCount(row.paintRow()) + len(row.uses)
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

func (c *canvas) rowElementCount(row ir.Row) int {
	count := len(c.backgroundSpans(row))
	for _, run := range row.Runs {
		if _, _, ok := compactTextRun(run); ok {
			count++
		}
	}
	return count
}

func semanticRowHash(row ir.Row) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	addInt := func(value int) {
		//nolint:gosec // convert signed values to stable two's-complement hash bytes.
		v := uint64(value)
		for range 8 {
			h = (h ^ uint64(byte(v))) * prime
			v >>= 8
		}
	}
	addInt(row.Y)
	addInt(len(row.Runs))
	for _, run := range row.Runs {
		addInt(len(run.Text))
		for i := range len(run.Text) {
			h = (h ^ uint64(run.Text[i])) * prime
		}
		addInt(run.StartCol)
		addInt(run.EndCol)
		addInt(int(run.Attrs.FG))
		addInt(int(run.Attrs.BG))
		flags := uint64(0)
		if run.Attrs.Bold {
			flags |= 1
		}
		if run.Attrs.Italic {
			flags |= 2
		}
		if run.Attrs.Underline {
			flags |= 4
		}
		if run.Attrs.Dim {
			flags |= 8
		}
		h = (h ^ flags) * prime
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
	if !c.stateNeedsWrapper(rows) {
		var body strings.Builder
		writer := c.w
		c.w = &body
		c.writeFrameRows(rows)
		c.w = writer
		fmt.Fprint(c.w, addElementID(body.String(), id))
		return
	}
	if c.stateElementCount(rows) == 0 && c.config.Minify {
		fmt.Fprintf(c.w, `<g id="%s"/>`, id)
		return
	}
	fmt.Fprintf(c.w, `<g id="%s">`, id)
	c.writeFrameRows(rows)
	fmt.Fprint(c.w, `</g>`)
}

func (c *canvas) stateNeedsWrapper(rows []*renderedRow) bool {
	if c.stateElementCount(rows) != 1 {
		return true
	}
	for _, row := range rows {
		if row.id != "" {
			return false
		}
		if c.renderedElementCount(row) == 1 {
			return elementIDPrefix(row.svg) == ""
		}
	}
	return true
}

func (c *canvas) stateElementCount(rows []*renderedRow) int {
	count := 0
	for _, row := range rows {
		if row.id != "" {
			count++
		} else {
			count += c.renderedElementCount(row)
		}
	}
	return count
}

func (c *canvas) writeContent(content *preparedContent) {
	if c.options.usesLocalViewports() {
		c.writeBands(content.bands)
		return
	}
	c.writeFrames(content.frameRows, content.frameKeyframes, content.frameStateIDs)
}

func (c *canvas) writeFrames(
	frameRows [][]*renderedRow,
	frames []keyframePoint[int],
	stateIDs []string,
) {
	if len(frames) <= 1 {
		if len(frameRows) > 0 {
			c.writeFrameRows(frameRows[len(frameRows)-1])
		}
		return
	}
	if c.options.FrameSwitch == FrameSwitchHref {
		c.writeHrefSequence(frames, stateIDs, "")
		return
	}
	if c.options.Animation == AnimationSMIL {
		fmt.Fprint(c.w, `<g>`)
		c.writeSMILTranslate(c.w, frames, c.contentWidth())
	} else {
		fmt.Fprintf(c.w, `<g style="animation:k %s %s step-end">`, animationDuration(c.plan.duration), c.loopCount())
	}
	c.writeStateStrip(frameRows, c.contentWidth())
	fmt.Fprint(c.w, "</g>")
}

func (c *canvas) writeBands(bands []preparedBand) {
	for bandIndex := range bands {
		c.writeBand(&bands[bandIndex])
	}
}

func (c *canvas) writeBand(band *preparedBand) {
	width := band.width * ColWidth
	height := band.height * RowHeight
	x, y := band.x*ColWidth, band.y*RowHeight
	xAttr, yAttr := ` x="`+c.xmlInt(x)+`"`, ` y="`+c.xmlInt(y)+`"`
	if band.kind == bandSnapshot && c.options.FrameSwitch == FrameSwitchHref && len(band.keyframes) > 1 {
		// An href-switched band has no strip to clip, so the animated use is
		// placed directly with the band offset instead of a nested viewport.
		position := ""
		if x != 0 {
			position += xAttr
		}
		if y != 0 {
			position += yAttr
		}
		c.writeHrefSequence(band.keyframes, band.stateIDs, position)
		return
	}
	if c.config.Minify && x == 0 {
		xAttr = ""
	}
	if c.config.Minify && y == 0 {
		yAttr = ""
	}
	fmt.Fprintf(c.w, `<svg%s%s width="%s" height="%s" overflow="hidden">`,
		xAttr, yAttr, c.xmlInt(width), c.xmlInt(height))
	if band.kind == bandScrollTape {
		if c.options.Animation == AnimationSMIL {
			fmt.Fprint(c.w, `<g>`)
			c.writeSMILVerticalTranslate(c.w, band.offsets)
		} else {
			fmt.Fprintf(c.w, `<g style="animation:%s %s %s step-end%s">`,
				band.name, animationDuration(c.plan.duration), c.loopCount(), c.scrollFiniteAnimationFill())
		}
		c.writeFrameRows(band.tapeRows)
		fmt.Fprint(c.w, `</g></svg>`)
		return
	}
	if band.kind == bandRetainedRect {
		c.writeRetainedRect(band.track)
		c.writeSnapshotBandBody(band, width)
		fmt.Fprint(c.w, `</svg>`)
		return
	}
	c.writeSnapshotBandBody(band, width)
	fmt.Fprint(c.w, `</svg>`)
}

func (c *canvas) writeSnapshotBandBody(band *preparedBand, width int) {
	if len(band.keyframes) <= 1 {
		if len(band.rows) > 0 {
			c.writeFrameRows(band.rows[len(band.rows)-1])
		}
		return
	}
	if c.options.FrameSwitch == FrameSwitchHref {
		c.writeHrefSequence(band.keyframes, band.stateIDs, "")
		return
	}
	if c.options.Animation == AnimationSMIL {
		fmt.Fprint(c.w, `<g>`)
		c.writeSMILTranslate(c.w, band.keyframes, width)
	} else {
		fmt.Fprintf(c.w, `<g style="animation:%s %s %s step-end%s">`,
			band.name, animationDuration(c.plan.duration), c.loopCount(), c.scrollFiniteAnimationFill())
	}
	c.writeStateStrip(band.rows, width)
	fmt.Fprint(c.w, `</g>`)
}

func (c *canvas) scrollFiniteAnimationFill() string {
	if c.options.Layout != LayoutScroll || infiniteLoop(c.config.LoopCount) {
		return ""
	}
	return " forwards"
}

// writeHrefSequence emits the animated use; position carries optional x/y
// attributes that offset the switched states.
func (c *canvas) writeHrefSequence(frames []keyframePoint[int], ids []string, position string) {
	if len(frames) == 0 || len(ids) == 0 {
		return
	}
	initial := frames[0].state
	if initial < 0 || initial >= len(ids) {
		return
	}
	fmt.Fprintf(c.w, `<use href="#%s"%s>`, ids[initial], position)
	c.writeSMILHref(c.w, frames, ids)
	fmt.Fprint(c.w, `</use>`)
}

func (c *canvas) writeStateStrip(states [][]*renderedRow, width int) {
	for i, rows := range states {
		if c.config.Minify && !renderedRowsHaveOutput(rows) {
			fmt.Fprintf(c.w, `<g transform="translate(%s)"/>`, c.xmlInt(width*i))
			continue
		}
		fmt.Fprintf(c.w, `<g transform="translate(%s)">`, c.xmlInt(width*i))
		c.writeFrameRows(rows)
		fmt.Fprint(c.w, `</g>`)
	}
}

func (c *canvas) generateBandKeyframes(name string, frames []keyframePoint[int], width int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "@keyframes %s{", name)
	for _, frame := range frames {
		fmt.Fprintf(&sb, "%s{transform:translateX(%dpx)}", frame.selector, -width*frame.state)
	}
	sb.WriteString("}")
	return sb.String()
}

func (c *canvas) generateScrollKeyframes(name string, frames []keyframePoint[int]) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "@keyframes %s{", name)
	for _, frame := range frames {
		fmt.Fprintf(&sb, "%s{transform:translateY(%dpx)}", frame.selector, -RowHeight*frame.state)
	}
	sb.WriteString("}")
	return sb.String()
}

func (c *canvas) writeFrameRows(rows []*renderedRow) {
	for _, row := range rows {
		if row.id != "" {
			fmt.Fprintf(c.w, `<use href="#%s"/>`, row.id)
		} else {
			fmt.Fprint(c.w, row.svg)
		}
	}
}

func renderedRowsHaveOutput(rows []*renderedRow) bool {
	for _, row := range rows {
		if row.id != "" || row.svg != "" {
			return true
		}
	}
	return false
}

func (c *canvas) writeRow(w io.Writer, row ir.Row) {
	for _, span := range c.backgroundSpans(row) {
		c.writeBackgroundSpan(w, row.Y, span)
	}
	for _, run := range row.Runs {
		c.writeTextRun(w, run, row.Y)
	}
}

func (c *canvas) writeBackgroundSpan(w io.Writer, rowY int, span backgroundSpan) {
	x := span.startCol * ColWidth
	xAttr := ""
	if x != 0 {
		xAttr = ` x="` + c.xmlInt(x) + `"`
	}
	if c.style.scheme == "" || c.style.scheme == styleLegacy {
		fmt.Fprintf(w, `<rect class="%s"%s y="%s" width="%s" height="%s"/>`,
			c.classNames[span.colorID], xAttr, c.xmlInt(rowY*RowHeight),
			c.xmlInt((span.endCol-span.startCol)*ColWidth), c.xmlInt(RowHeight))
		return
	}
	fmt.Fprintf(w, `<rect%s%s y="%s" width="%s" height="%s"/>`,
		styleAttributes(c.style.backgrounds[span.colorID]), xAttr, c.xmlInt(rowY*RowHeight),
		c.xmlInt((span.endCol-span.startCol)*ColWidth), c.xmlInt(RowHeight))
}

func (c *canvas) backgroundSpans(row ir.Row) []backgroundSpan {
	spans := make([]backgroundSpan, 0, len(row.Runs))
	for i, run := range row.Runs {
		endCol := runEndCol(run)
		if c.rec.Colors.IsDefault(run.Attrs.BG) || endCol <= run.StartCol {
			continue
		}
		if len(spans) > 0 && spans[len(spans)-1].colorID == run.Attrs.BG && spans[len(spans)-1].endCol == run.StartCol {
			spans[len(spans)-1].endCol = endCol
			spans[len(spans)-1].lastRun = i
			continue
		}
		spans = append(spans, backgroundSpan{
			startCol: run.StartCol, endCol: endCol, colorID: run.Attrs.BG, firstRun: i, lastRun: i,
		})
	}
	return spans
}

func runEndCol(run ir.TextRun) int {
	if run.EndCol > run.StartCol {
		return run.EndCol
	}
	return run.StartCol + utf8.RuneCountInString(run.Text)
}

func shouldRenderText(run ir.TextRun) bool {
	_, _, ok := compactTextRun(run)
	return ok
}

// compactTextRun removes only visually inert ASCII spaces. Backgrounds are
// emitted independently, and underlined whitespace remains visible. Other
// Unicode whitespace is intentionally preserved.
func compactTextRun(run ir.TextRun) (text string, startCol int, ok bool) {
	text = run.Text
	startCol = run.StartCol
	if !run.Attrs.Underline {
		trimmedLeft := strings.TrimLeft(text, " ")
		startCol += len(text) - len(trimmedLeft)
		text = strings.TrimRight(trimmedLeft, " ")
	}
	return text, startCol, text != ""
}

func (c *canvas) writeCursor() {
	if !c.plan.cursorEverVisible {
		return
	}
	point := c.plan.cursor.points[len(c.plan.cursor.points)-1]
	style := ""
	frames := c.cursorKeyframes()
	if len(frames) > 0 {
		point.state = frames[0].state
	}
	if len(frames) > 1 && c.options.Animation == AnimationCSS {
		style = fmt.Sprintf(` style="animation:cursor %s %s step-end%s"`,
			animationDuration(c.plan.duration), c.loopCount(), c.scrollFiniteAnimationFill())
	}
	fmt.Fprintf(c.w, `<g transform="translate(%s,%s)" visibility="%s"%s>`,
		c.xmlInt(point.state.Col*ColWidth), c.xmlInt(point.state.Row*RowHeight), cursorVisibility(point.state), style)
	if len(frames) > 1 && c.options.Animation == AnimationSMIL {
		c.writeSMILCursor(c.w, frames)
	}
	cursorClass := "cursor"
	if c.style.scheme != "" && c.style.scheme != styleLegacy {
		cursorClass = c.style.cursorClass
	}
	fmt.Fprintf(c.w, `<rect class="%s" width="%s" height="%s"/></g>`, cursorClass, c.xmlInt(ColWidth), c.xmlInt(RowHeight))
}

func (c *canvas) writeTextRun(w io.Writer, run ir.TextRun, rowY int) {
	text, startCol, ok := compactTextRun(run)
	if !ok {
		return
	}

	// Replace spaces with non-breaking spaces to survive minification. The
	// streaming export transform restores them after minification.
	if c.config.Minify {
		text = strings.ReplaceAll(text, " ", "\u00A0")
	}

	x := startCol * ColWidth
	y := (rowY*RowHeight + RowHeight) - 5 // baseline offset
	xAttr := ""
	if x != 0 {
		xAttr = ` x="` + c.xmlInt(x) + `"`
	}

	if c.style.scheme != "" && c.style.scheme != styleLegacy {
		key := textStyleKey{run.Attrs.FG, run.Attrs.Bold, run.Attrs.Italic, run.Attrs.Underline, run.Attrs.Dim}
		fmt.Fprintf(w, `<text%s y="%s"%s>%s</text>`,
			xAttr, c.xmlInt(y), styleAttributes(c.style.texts[key]), svgTextEscaper.Replace(text))
		return
	}

	// Build class list
	var classes []string
	if !c.rec.Colors.IsDefault(run.Attrs.FG) {
		classes = append(classes, c.classNames[run.Attrs.FG])
	}
	if run.Attrs.Bold {
		classes = append(classes, "bold")
	}
	if run.Attrs.Italic {
		classes = append(classes, "italic")
	}
	if run.Attrs.Underline {
		classes = append(classes, "underline")
	}
	if run.Attrs.Dim {
		classes = append(classes, "dim")
	}

	// Build attributes
	classAttr := ""
	if len(classes) > 0 {
		classAttr = fmt.Sprintf(" class=%q", strings.Join(classes, " "))
	}

	fmt.Fprintf(w, `<text%s y="%s"%s>%s</text>`, xAttr, c.xmlInt(y), classAttr, svgTextEscaper.Replace(text))
}

func (c *canvas) xmlInt(value int) string {
	plain := strconv.Itoa(value)
	if !c.config.Minify || value == 0 {
		return plain
	}
	sign, digits := "", plain
	if digits[0] == '-' {
		sign, digits = "-", digits[1:]
	}
	zeros := len(digits) - len(strings.TrimRight(digits, "0"))
	if zeros == 0 {
		return plain
	}
	scientific := sign + strings.TrimRight(digits, "0") + "e" + strconv.Itoa(zeros)
	if len(scientific) < len(plain) {
		return scientific
	}
	return plain
}
