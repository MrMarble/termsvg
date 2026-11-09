package svg

import (
	"fmt"
	"strings"
)

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
