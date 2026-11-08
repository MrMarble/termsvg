package svg

import (
	"slices"

	"github.com/mrmarble/termsvg/pkg/ir"
)

type rowLane struct {
	y       int
	content timeline[ir.Row]
}

type rowBand struct {
	x       int
	y       int
	width   int
	height  int
	content timeline[[]ir.Row]
}

func buildRowBands(plan *renderPlan, width, height int) []rowBand {
	lanes := make([]rowLane, 0, height)
	for y := range height {
		points := make([]timelinePoint[ir.Row], len(plan.content.points))
		for i, point := range plan.content.points {
			points[i] = timelinePoint[ir.Row]{time: point.time, state: rowAt(point.state, y)}
		}
		content := normalizeTimeline(plan.duration, points, rowEqual)
		if len(content.points) > 1 {
			lanes = append(lanes, rowLane{y: y, content: content})
		}
	}

	bands := make([]rowBand, 0, len(lanes))
	for start := 0; start < len(lanes); {
		end := start + 1
		for end < len(lanes) && lanes[end].y == lanes[end-1].y+1 &&
			timelineTimesEqual(lanes[start].content, lanes[end].content) {
			end++
		}
		band := combineRowLanes(lanes[start:end])
		band.localize(width)
		bands = append(bands, band)
		start = end
	}
	return bands
}

func timelineTimesEqual[T, U any](a timeline[T], b timeline[U]) bool {
	return a.duration == b.duration && len(a.points) == len(b.points) &&
		slices.EqualFunc(a.points, b.points, func(a timelinePoint[T], b timelinePoint[U]) bool {
			return a.time == b.time
		})
}

func combineRowLanes(lanes []rowLane) rowBand {
	startY := lanes[0].y
	points := make([]timelinePoint[[]ir.Row], len(lanes[0].content.points))
	for i, point := range lanes[0].content.points {
		rows := make([]ir.Row, 0, len(lanes))
		for _, lane := range lanes {
			row := lane.content.points[i].state
			if len(row.Runs) == 0 {
				continue
			}
			row.Runs = slices.Clone(row.Runs)
			row.Y = lane.y - startY
			rows = append(rows, row)
		}
		points[i] = timelinePoint[[]ir.Row]{time: point.time, state: rows}
	}
	return rowBand{
		y:      startY,
		height: len(lanes),
		content: timeline[[]ir.Row]{
			duration: lanes[0].content.duration,
			points:   points,
		},
	}
}

func (b *rowBand) localize(terminalWidth int) {
	minCol, maxCol, ok := bandColumnBounds(b.content.points)
	if !ok {
		b.width = max(1, terminalWidth)
		return
	}
	// Keep one cell of transparent overhang on each available side. This
	// prevents italic or otherwise overhanging glyphs from being clipped while
	// still reducing the transformed surface for narrow TUI updates.
	minCol = max(0, minCol-1)
	maxCol = min(terminalWidth, maxCol+1)
	b.x = minCol
	b.width = max(1, maxCol-minCol)
	if minCol == 0 {
		return
	}
	for i := range b.content.points {
		for rowIndex := range b.content.points[i].state {
			for runIndex := range b.content.points[i].state[rowIndex].Runs {
				run := &b.content.points[i].state[rowIndex].Runs[runIndex]
				run.StartCol -= minCol
				if run.EndCol > 0 {
					run.EndCol -= minCol
				}
			}
		}
	}
}

func bandColumnBounds(points []timelinePoint[[]ir.Row]) (minCol, maxCol int, ok bool) {
	for _, point := range points {
		for _, row := range point.state {
			for _, run := range row.Runs {
				end := runEndCol(run)
				if end <= run.StartCol {
					continue
				}
				if !ok || run.StartCol < minCol {
					minCol = run.StartCol
				}
				if !ok || end > maxCol {
					maxCol = end
				}
				ok = true
			}
		}
	}
	return minCol, maxCol, ok
}
