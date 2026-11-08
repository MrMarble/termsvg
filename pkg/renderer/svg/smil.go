package svg

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
)

type smilAttribute struct {
	name  string
	value string
}

func (c *canvas) writeSMILTranslate(w io.Writer, frames []keyframePoint[int], width int) {
	values := make([]string, len(frames))
	for i, frame := range frames {
		values[i] = strconv.Itoa(-width * frame.state)
	}
	c.writeSMILAnimation(w, "animateTransform", []smilAttribute{
		{name: "attributeName", value: "transform"},
		{name: "type", value: string(FrameSwitchTranslate)},
		{name: "values", value: strings.Join(values, ";")},
		{name: "keyTimes", value: smilKeyTimes(frames)},
	})
}

func (c *canvas) writeSMILVerticalTranslate(w io.Writer, frames []keyframePoint[int]) {
	values := make([]string, len(frames))
	for i, frame := range frames {
		values[i] = "0 " + strconv.Itoa(-RowHeight*frame.state)
	}
	c.writeSMILAnimation(w, "animateTransform", []smilAttribute{
		{name: "attributeName", value: "transform"},
		{name: "type", value: string(FrameSwitchTranslate)},
		{name: "values", value: strings.Join(values, ";")},
		{name: "keyTimes", value: smilKeyTimes(frames)},
	})
}

func (c *canvas) writeSMILHref(w io.Writer, frames []keyframePoint[int], ids []string) {
	values := make([]string, len(frames))
	for i, frame := range frames {
		if frame.state < 0 || frame.state >= len(ids) {
			return
		}
		values[i] = "#" + ids[frame.state]
	}
	c.writeSMILAnimation(w, "animate", []smilAttribute{
		{name: "attributeName", value: "href"},
		{name: "values", value: strings.Join(values, ";")},
		{name: "keyTimes", value: smilKeyTimes(frames)},
	})
}

func (c *canvas) writeSMILCursor(w io.Writer, frames []keyframePoint[ir.Cursor]) {
	positionChanges := false
	visibilityChanges := false
	for i := 1; i < len(frames); i++ {
		positionChanges = positionChanges || frames[i-1].state.Col != frames[i].state.Col ||
			frames[i-1].state.Row != frames[i].state.Row
		visibilityChanges = visibilityChanges || frames[i-1].state.Visible != frames[i].state.Visible
	}

	if positionChanges {
		values := make([]string, len(frames))
		for i, frame := range frames {
			values[i] = fmt.Sprintf("%d %d", frame.state.Col*ColWidth, frame.state.Row*RowHeight)
		}
		c.writeSMILAnimation(w, "animateTransform", []smilAttribute{
			{name: "attributeName", value: "transform"},
			{name: "type", value: "translate"},
			{name: "values", value: strings.Join(values, ";")},
			{name: "keyTimes", value: smilKeyTimes(frames)},
		})
	}
	if visibilityChanges {
		values := make([]string, len(frames))
		for i, frame := range frames {
			values[i] = cursorVisibility(frame.state)
		}
		c.writeSMILAnimation(w, "animate", []smilAttribute{
			{name: "attributeName", value: "visibility"},
			{name: "values", value: strings.Join(values, ";")},
			{name: "keyTimes", value: smilKeyTimes(frames)},
		})
	}
}

func (c *canvas) writeSMILAnimation(w io.Writer, element string, attributes []smilAttribute) {
	fmt.Fprintf(w, "<%s", element)
	for _, attribute := range attributes {
		fmt.Fprintf(w, ` %s="%s"`, attribute.name, attribute.value)
	}
	fill := ""
	if !infiniteLoop(c.config.LoopCount) {
		fill = ` fill="freeze"`
	}
	fmt.Fprintf(w, ` calcMode="discrete" dur="%s" repeatCount="%s"%s/>`,
		smilAnimationDuration(c.plan.duration), c.smilRepeatCount(), fill)
}

func smilAnimationDuration(duration time.Duration) string {
	value := animationDuration(duration)
	if strings.HasPrefix(value, ".") {
		return "0" + value
	}
	if strings.HasPrefix(value, "-.") {
		return "-0" + strings.TrimPrefix(value, "-")
	}
	return value
}

func smilKeyTimes[T any](frames []keyframePoint[T]) string {
	values := make([]string, len(frames))
	for i, frame := range frames {
		values[i] = percentageToUnitInterval(frame.selector)
	}
	return strings.Join(values, ";")
}

// percentageToUnitInterval divides an already-normalized decimal percentage by
// 100 without routing it through float64 again.
func percentageToUnitInterval(selector string) string {
	percentage := strings.TrimSuffix(selector, "%")
	if percentage == "0" {
		return "0"
	}
	if percentage == "100" {
		return "1"
	}

	whole, fraction, found := strings.Cut(percentage, ".")
	if !found {
		fraction = ""
	}
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return "0"
	}
	scale := len(fraction) + 2
	if len(digits) <= scale {
		value := "." + strings.Repeat("0", scale-len(digits)) + digits
		return strings.TrimRight(value, "0")
	}
	pivot := len(digits) - scale
	value := digits[:pivot] + "." + digits[pivot:]
	value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	return value
}

func (c *canvas) smilRepeatCount() string {
	if c.config.LoopCount == -1 {
		return "1"
	}
	if c.config.LoopCount > 0 {
		return strconv.Itoa(c.config.LoopCount)
	}
	return "indefinite"
}
