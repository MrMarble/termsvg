package svg

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/layout"
	"github.com/mrmarble/termsvg/internal/renderer"
)

// canvas is the state of rendering one candidate: the recording, its plan, the
// options and config, and the colour classes and style plan chosen for it.
//
// It writes the document (w is nil while only preparing or measuring) and
// prepares content, which measures candidates by serializing them through probe
// copies of the canvas, so the two phases stay together. Everything that needs
// only part of this state lives in a narrower collaborator built on demand:
// rows() for row markup, styles() for style planning, collector() for shared
// row definitions and smil() for SMIL animation elements.
type canvas struct {
	w          io.Writer
	rec        *ir.Recording
	plan       semanticPlan
	config     renderer.Config
	options    Options
	classNames map[color.ID]string
	style      stylePlan
	metrics    *CandidateMetrics
}

func newCanvas(rec *ir.Recording, plan *semanticPlan, config *renderer.Config, options *Options) *canvas {
	return &canvas{
		rec:        rec,
		plan:       *plan,
		config:     *config,
		options:    *options,
		classNames: rec.Colors.GenerateClassNames(),
		style:      stylePlan{scheme: styleLegacy},
	}
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
	return layout.PaddedWidth(c.contentWidth())
}

func (c *canvas) paddedHeight() int {
	return layout.PaddedHeight(c.contentHeight(), c.config.ShowWindow)
}

// docSection is one fragment of the document. bytes, when set, is the already
// measured size of the fragment, so the cost ledger need not write it again.
type docSection struct {
	write func(io.Writer)
	bytes *int64
}

// documentSections lists the document in order. render writes it and
// buildPreparedCandidateCost measures it, so both always cover the same bytes.
func (c *canvas) documentSections(content *preparedContent) []docSection {
	at := func(write func()) func(io.Writer) {
		return func(w io.Writer) { c.w = w; write() }
	}
	text := func(s string) docSection {
		return docSection{write: func(w io.Writer) { fmt.Fprint(w, s) }}
	}
	contentY := layout.ContentOffsetY(c.config.ShowWindow)
	opening := at(c.writeBackground)
	if c.config.ShowWindow {
		opening = at(c.writeWindow)
	}
	sections := []docSection{
		{write: at(c.writeSVGOpen)},
		{write: opening},
		{write: func(w io.Writer) {
			fmt.Fprintf(w, `<defs><clipPath id="clip"><rect width="%s" height="%s"/></clipPath>`,
				c.xmlInt(c.contentWidth()), c.xmlInt(c.contentHeight()))
		}},
		{write: at(func() { c.writeRowDefs(content.rowDefs); c.writeStateDefs(content) }), bytes: &content.cost.definitions},
		text(`</defs>`),
		{write: at(func() { c.writeContentGroupOpen(contentY) })},
		{write: at(func() { c.writeStyles(content) }), bytes: &content.cost.styles},
	}
	for _, row := range c.plan.staticRows {
		sections = append(sections, docSection{write: func(w io.Writer) { c.rows().writeRow(w, row) }})
	}
	for index := range c.plan.layers {
		sections = append(sections, docSection{write: func(w io.Writer) { c.writeLayer(w, index) }})
	}
	return append(sections,
		docSection{write: at(func() { c.writeContent(content) }), bytes: &content.cost.active},
		docSection{write: at(c.writeCursor)},
		text(`</g></svg>`),
	)
}

func (c *canvas) render(ctx context.Context, content *preparedContent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	for _, section := range c.documentSections(content) {
		section.write(c.w)
	}
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

	bgHex := color.RGBAtoHex(theme.WindowBackground)
	fmt.Fprintf(c.w, `<rect rx="%s" width="100%%" height="100%%" fill="%s"/>`, c.xmlInt(windowCornerRadius), bgHex)

	buttonY := Padding
	for i, btnColor := range theme.WindowButtons {
		btnHex := color.RGBAtoHex(btnColor)
		x := Padding + i*layout.ButtonSpacing
		fmt.Fprintf(c.w, `<circle cx="%s" cy="%s" r="%s" fill="%s"/>`,
			c.xmlInt(x), c.xmlInt(buttonY), c.xmlInt(layout.ButtonRadius), btnHex)
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
		if len(c.plan.cursorKeyframes()) > 1 {
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
	if c.style.legacy() {
		legacy := c.styles().legacyStylePlan()
		fmt.Fprintf(sb, "text{%s}", legacy.textBaseRule)
		if legacy.cursorRule != "" {
			fmt.Fprintf(sb, ".cursor{%s}", legacy.cursorRule)
		}
		for _, id := range c.plan.usedColors {
			fmt.Fprintf(sb, ".%s{fill:%s}", c.classNames[id], color.RGBAtoHex(c.rec.Colors.Resolved(id)))
		}
		used := [4]bool{c.rec.Stats.HasBold, c.rec.Stats.HasItalic, c.rec.Stats.HasUnderline, c.rec.Stats.HasDim}
		for i, attr := range textAttrs {
			if used[i] {
				fmt.Fprintf(sb, ".%s{%s}", attr.class, attr.declaration)
			}
		}
		return
	}
	fmt.Fprintf(sb, "text{%s}", c.style.textBaseRule)
	for _, rule := range c.style.rules {
		fmt.Fprintf(sb, ".%s{%s}", rule.class, rule.declarations)
	}
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

// rows returns a rowWriter that reads the canvas's current recording, colour
// classes and style plan.
func (c *canvas) rows() rowWriter {
	return rowWriter{rec: c.rec, classNames: c.classNames, style: &c.style, minify: c.config.Minify}
}

func (c *canvas) styles() stylePlanner {
	return stylePlanner{rec: c.rec, config: &c.config, plan: &c.plan}
}

func (c *canvas) collector() rowCollector {
	return rowCollector{rows: c.rows(), withoutBlockSharing: c.options.withoutBlockSharing}
}

func (c *canvas) smil() smilWriter {
	return smilWriter{loopCount: loopCount(c.config.LoopCount), duration: c.plan.duration}
}

func (c *canvas) xmlInt(value int) string { return formatInt(value, c.config.Minify) }

// formatInt formats an integer attribute value, using scientific notation when
// that is shorter and the output is minified.
func formatInt(value int, minify bool) string {
	plain := strconv.Itoa(value)
	if !minify || value == 0 {
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
