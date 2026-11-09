package svg

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
)

// rowWriter serializes terminal rows (background rectangles and text runs) to
// SVG markup. It carries only what row painting reads, so rows can be written
// without a canvas.
type rowWriter struct {
	rec        *ir.Recording
	classNames map[color.ID]string
	style      *stylePlan
	minify     bool
}

type backgroundSpan struct {
	startCol int
	endCol   int
	colorID  color.ID
	firstRun int
	lastRun  int
}

var svgTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// xmlInt formats an integer attribute value; see formatInt.
func (rw rowWriter) xmlInt(value int) string { return formatInt(value, rw.minify) }

func (rw rowWriter) writeRow(w io.Writer, row ir.Row) {
	for _, span := range rw.backgroundSpans(row) {
		rw.writeBackgroundSpan(w, row.Y, span)
	}
	for _, run := range row.Runs {
		rw.writeTextRun(w, run, row.Y)
	}
}

func (rw rowWriter) writeBackgroundSpan(w io.Writer, rowY int, span backgroundSpan) {
	x := span.startCol * ColWidth
	xAttr := ""
	if x != 0 {
		xAttr = ` x="` + rw.xmlInt(x) + `"`
	}
	if rw.style.scheme == "" || rw.style.scheme == styleLegacy {
		fmt.Fprintf(w, `<rect class="%s"%s y="%s" width="%s" height="%s"/>`,
			rw.classNames[span.colorID], xAttr, rw.xmlInt(rowY*RowHeight),
			rw.xmlInt((span.endCol-span.startCol)*ColWidth), rw.xmlInt(RowHeight))
		return
	}
	fmt.Fprintf(w, `<rect%s%s y="%s" width="%s" height="%s"/>`,
		styleAttributes(rw.style.backgrounds[span.colorID]), xAttr, rw.xmlInt(rowY*RowHeight),
		rw.xmlInt((span.endCol-span.startCol)*ColWidth), rw.xmlInt(RowHeight))
}

func (rw rowWriter) backgroundSpans(row ir.Row) []backgroundSpan {
	spans := make([]backgroundSpan, 0, len(row.Runs))
	for i, run := range row.Runs {
		endCol := runEndCol(run)
		if rw.rec.Colors.IsDefault(run.Attrs.BG) || endCol <= run.StartCol {
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

func (rw rowWriter) writeTextRun(w io.Writer, run ir.TextRun, rowY int) {
	text, startCol, ok := compactTextRun(run)
	if !ok {
		return
	}

	// Replace spaces with non-breaking spaces to survive minification. The
	// streaming export transform restores them after minification.
	if rw.minify {
		text = strings.ReplaceAll(text, " ", "\u00A0")
	}

	x := startCol * ColWidth
	y := (rowY*RowHeight + RowHeight) - 5 // baseline offset
	xAttr := ""
	if x != 0 {
		xAttr = ` x="` + rw.xmlInt(x) + `"`
	}

	if rw.style.scheme != "" && rw.style.scheme != styleLegacy {
		key := textStyleKey{run.Attrs.FG, run.Attrs.Bold, run.Attrs.Italic, run.Attrs.Underline, run.Attrs.Dim}
		fmt.Fprintf(w, `<text%s y="%s"%s>%s</text>`,
			xAttr, rw.xmlInt(y), styleAttributes(rw.style.texts[key]), svgTextEscaper.Replace(text))
		return
	}

	// Build class list
	var classes []string
	if !rw.rec.Colors.IsDefault(run.Attrs.FG) {
		classes = append(classes, rw.classNames[run.Attrs.FG])
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

	fmt.Fprintf(w, `<text%s y="%s"%s>%s</text>`, xAttr, rw.xmlInt(y), classAttr, svgTextEscaper.Replace(text))
}
