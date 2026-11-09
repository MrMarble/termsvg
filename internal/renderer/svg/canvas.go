package svg

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/renderer"
)

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
