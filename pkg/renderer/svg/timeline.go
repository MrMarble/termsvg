package svg

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
)

type timelinePoint[T any] struct {
	time  time.Duration
	state T
}

type timeline[T any] struct {
	duration time.Duration
	points   []timelinePoint[T]
}

type keyframePoint[T any] struct {
	selector string
	state    T
}

const maxTimelinePrecision = 12

func normalizeTimeline[T any](duration time.Duration, points []timelinePoint[T], equal func(T, T) bool) timeline[T] {
	if len(points) == 0 {
		return timeline[T]{duration: duration}
	}
	points = append([]timelinePoint[T](nil), points...)
	slices.SortStableFunc(points, func(a, b timelinePoint[T]) int {
		if a.time < b.time {
			return -1
		}
		if a.time > b.time {
			return 1
		}
		return 0
	})
	for i := range points {
		points[i].time = max(0, min(points[i].time, duration))
	}

	out := points[:0]
	for _, point := range points {
		if len(out) > 0 && out[len(out)-1].time == point.time {
			out[len(out)-1] = point
		} else {
			out = append(out, point)
		}
	}
	compacted := out[:0]
	for _, point := range out {
		if len(compacted) == 0 || !equal(compacted[len(compacted)-1].state, point.state) {
			compacted = append(compacted, point)
		}
	}
	out = compacted
	out[0].time = 0
	if len(out) > 1 && out[len(out)-1].time < duration {
		out = append(out, timelinePoint[T]{time: duration, state: out[len(out)-1].state})
	}
	return timeline[T]{duration: duration, points: out}
}

// quantizeTimeline samples a normalized timeline at fixed bucket ends. The
// last state observed in each bucket wins. The initial and final states are
// always preserved, so a recording shorter than one bucket can contain both.
func quantizeTimeline[T any](t timeline[T], maxFPS int, equal func(T, T) bool) timeline[T] {
	if maxFPS <= 0 || t.duration <= 0 || len(t.points) <= 1 {
		return t
	}
	step := time.Second / time.Duration(maxFPS)
	if step <= 0 {
		return t
	}

	out := make([]timelinePoint[T], 1, len(t.points))
	out[0] = t.points[0]
	var pending timelinePoint[T]
	var pendingEnd time.Duration
	hasPending := false
	flush := func() {
		if hasPending {
			out = append(out, pending)
			hasPending = false
		}
	}

	for _, point := range t.points[1:] {
		end := timelineBucketEnd(point.time, step, t.duration)
		if hasPending && end != pendingEnd {
			flush()
		}
		pending = timelinePoint[T]{time: end, state: point.state}
		pendingEnd = end
		hasPending = true
	}
	flush()

	return normalizeTimeline(t.duration, out, equal)
}

func timelineBucketEnd(pointTime, step, duration time.Duration) time.Duration {
	if pointTime <= 0 {
		return 0
	}
	if pointTime >= duration {
		return duration
	}
	remainder := pointTime % step
	if remainder == 0 {
		return pointTime
	}
	advance := step - remainder
	if advance >= duration-pointTime {
		return duration
	}
	return pointTime + advance
}

func (t timeline[T]) animated() bool {
	return t.duration > 0 && len(t.points) > 1
}

func (t timeline[T]) keyframes(equal ...func(T, T) bool) []keyframePoint[T] {
	if !t.animated() {
		return nil
	}
	for precision := 3; precision <= maxTimelinePrecision; precision++ {
		frames := t.keyframesAtPrecision(precision)
		unique := true
		for i := 1; i < len(frames); i++ {
			if frames[i-1].selector == frames[i].selector {
				unique = false
				break
			}
		}
		if unique {
			return frames
		}
	}
	frames := t.keyframesAtPrecision(maxTimelinePrecision)
	out := frames[:0]
	for _, frame := range frames {
		if len(out) > 0 && out[len(out)-1].selector == frame.selector {
			out[len(out)-1] = frame
		} else {
			out = append(out, frame)
		}
	}
	if len(equal) > 0 {
		compacted := out[:0]
		for _, frame := range out {
			if len(compacted) == 0 || !equal[0](compacted[len(compacted)-1].state, frame.state) {
				compacted = append(compacted, frame)
			}
		}
		out = compacted
	}
	return out
}

func (t timeline[T]) keyframesAtPrecision(precision int) []keyframePoint[T] {
	frames := make([]keyframePoint[T], len(t.points))
	for i, point := range t.points {
		var selector string
		//nolint:exhaustive // all non-boundary durations use the percentage case.
		switch point.time {
		case 0:
			selector = "0%"
		case t.duration:
			selector = "100%"
		default:
			selector = strings.TrimRight(strings.TrimRight(
				strconv.FormatFloat(float64(point.time)*100/float64(t.duration), 'f', precision, 64), "0"), ".") + "%"
		}
		frames[i] = keyframePoint[T]{selector: selector, state: point.state}
	}
	return frames
}

func animationDuration(duration time.Duration) string {
	seconds := decimalDuration(duration, time.Second, "s")
	milliseconds := decimalDuration(duration, time.Millisecond, "ms")
	if len(seconds) <= len(milliseconds) {
		return seconds
	}
	return milliseconds
}

func decimalDuration(duration, unit time.Duration, suffix string) string {
	prefix := ""
	//nolint:gosec // duration is converted as unsigned magnitude below.
	value := uint64(duration)
	if duration < 0 {
		prefix = "-"
		//nolint:gosec // avoids overflow when duration is the minimum int64.
		value = uint64(-(duration + 1)) + 1
	}
	//nolint:gosec // time.Duration units are positive.
	whole, fraction := value/uint64(unit), value%uint64(unit)
	if fraction == 0 {
		return prefix + strconv.FormatUint(whole, 10) + suffix
	}
	digits := len(strconv.FormatInt(int64(unit), 10)) - 1
	formatted := strconv.FormatUint(whole, 10) + "." + fmt.Sprintf("%0*d", digits, fraction)
	formatted = strings.TrimRight(formatted, "0")
	return prefix + strings.TrimPrefix(formatted, "0") + suffix
}

func (c *canvas) loopCount() string {
	if c.config.LoopCount == -1 {
		return "1"
	}
	if c.config.LoopCount > 0 {
		return fmt.Sprintf("%d", c.config.LoopCount)
	}
	return "infinite"
}

func (c *canvas) generateCursorKeyframes() string {
	var sb strings.Builder
	sb.WriteString("@keyframes cursor{")
	for _, point := range c.cursorKeyframes() {
		fmt.Fprintf(&sb, "%s{transform:translate(%dpx,%dpx);visibility:%s}", point.selector,
			point.state.Col*ColWidth, point.state.Row*RowHeight, cursorVisibility(point.state))
	}
	sb.WriteString("}")
	return sb.String()
}

func (c *canvas) cursorKeyframes() []keyframePoint[ir.Cursor] {
	return c.plan.cursor.keyframes(cursorStatesEqual)
}

func cursorStatesEqual(a, b ir.Cursor) bool { return a == b }

func cursorVisibility(cursor ir.Cursor) string {
	if cursor.Visible {
		return "visible"
	}
	return "hidden"
}
