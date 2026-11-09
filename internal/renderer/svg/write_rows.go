package svg

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
)

type backgroundSpan struct {
	startCol int
	endCol   int
	colorID  color.ID
	firstRun int
	lastRun  int
}

var svgTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

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
