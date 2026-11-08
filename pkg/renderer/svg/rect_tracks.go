package svg

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

type retainedRectTrack struct {
	x, width []keyframePoint[int]
	fill     []keyframePoint[color.ID]
}

func (c *canvas) retainedRectCandidate(band rowBand) (retainedRectTrack, timeline[[]ir.Row], bool) {
	if band.height != 1 {
		return retainedRectTrack{}, timeline[[]ir.Row]{}, false
	}
	keyframes, states := contentKeyframesFor(band.content)
	spans := make([]*backgroundSpan, len(states))
	for i, state := range states {
		if len(state) > 1 {
			return retainedRectTrack{}, timeline[[]ir.Row]{}, false
		}
		var row ir.Row
		if len(state) == 1 {
			row = state[0]
		}
		backgrounds := c.backgroundSpans(row)
		if len(backgrounds) > 1 {
			return retainedRectTrack{}, timeline[[]ir.Row]{}, false
		}
		if len(backgrounds) == 1 {
			span := backgrounds[0]
			spans[i] = &span
		}
	}
	// The rectangle keeps its last position and colour while it is absent,
	// so the carry follows the keyframes in time order, not the state order.
	var first *backgroundSpan
	for _, frame := range keyframes {
		if spans[frame.state] != nil {
			first = spans[frame.state]
			break
		}
	}
	if first == nil {
		return retainedRectTrack{}, timeline[[]ir.Row]{}, false
	}
	last := *first
	track := retainedRectTrack{
		x: make([]keyframePoint[int], len(keyframes)), width: make([]keyframePoint[int], len(keyframes)),
		fill: make([]keyframePoint[color.ID], len(keyframes)),
	}
	for i, frame := range keyframes {
		width := 0
		if span := spans[frame.state]; span != nil {
			last = *span
			width = (span.endCol - span.startCol) * ColWidth
		}
		track.x[i] = keyframePoint[int]{selector: frame.selector, state: last.startCol * ColWidth}
		track.width[i] = keyframePoint[int]{selector: frame.selector, state: width}
		track.fill[i] = keyframePoint[color.ID]{selector: frame.selector, state: last.colorID}
	}
	return track, stripBackgrounds(band.content), true
}

func stripBackgrounds(content timeline[[]ir.Row]) timeline[[]ir.Row] {
	out := content
	out.points = slices.Clone(content.points)
	for i := range out.points {
		out.points[i].state = slices.Clone(content.points[i].state)
		for j := range out.points[i].state {
			out.points[i].state[j].Runs = slices.Clone(content.points[i].state[j].Runs)
			for k := range out.points[i].state[j].Runs {
				out.points[i].state[j].Runs[k].Attrs.BG = color.DefaultID
			}
		}
	}
	return out
}

func timelineChanges[T comparable](points []keyframePoint[T]) bool {
	for i := 1; i < len(points); i++ {
		if points[i-1].state != points[i].state {
			return true
		}
	}
	return false
}

func (t *retainedRectTrack) geometryAnimationCount() int {
	return boolInt(timelineChanges(t.x)) + boolInt(timelineChanges(t.width))
}

func (t *retainedRectTrack) fillChanges() bool { return timelineChanges(t.fill) }
func (t *retainedRectTrack) animationCount() int {
	return t.geometryAnimationCount() + boolInt(t.fillChanges())
}

func (c *canvas) writeRetainedRect(track *retainedRectTrack) {
	if track == nil || len(track.width) == 0 {
		return
	}
	initialX, initialWidth, initialFill := track.x[0].state, track.width[0].state, track.fill[0].state
	x := ""
	if initialX != 0 {
		x = ` x="` + c.xmlInt(initialX) + `"`
	}
	paint := ` class="` + c.classNames[initialFill] + `"`
	if c.style.scheme != "" && c.style.scheme != styleLegacy {
		paint = styleAttributes(c.style.backgrounds[initialFill])
	}
	fmt.Fprintf(c.w, `<rect%s%s y="0" width="%s" height="%s">`, paint, x, c.xmlInt(initialWidth), c.xmlInt(RowHeight))
	if timelineChanges(track.x) {
		writeRetainedRectAnimation(c, c.w, "x", track.x, strconv.Itoa)
	}
	if timelineChanges(track.width) {
		writeRetainedRectAnimation(c, c.w, "width", track.width, strconv.Itoa)
	}
	if track.fillChanges() {
		writeRetainedRectAnimation(c, c.w, "fill", track.fill, func(value color.ID) string {
			return c.paintHex(c.rec.Colors.Resolved(value))
		})
	}
	fmt.Fprint(c.w, `</rect>`)
}

func writeRetainedRectAnimation[T any](
	c *canvas,
	w io.Writer,
	name string,
	points []keyframePoint[T],
	format func(T) string,
) {
	values := make([]string, len(points))
	for i, point := range points {
		values[i] = format(point.state)
	}
	c.writeSMILAnimation(w, "animate", []smilAttribute{
		{name: "attributeName", value: name},
		{name: "values", value: strings.Join(values, ";")},
		{name: "keyTimes", value: smilKeyTimes(points)},
	})
}
