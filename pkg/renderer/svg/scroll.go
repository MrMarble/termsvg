package svg

import (
	"slices"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

type scrollTape struct {
	rows    []ir.Row
	offsets []keyframePoint[int]
}

func detectUpwardScrollTape(band rowBand, colors *color.Catalog) (scrollTape, bool) {
	keyframes, states := contentKeyframesFor(band.content)
	if band.height < 2 || len(states) < 2 {
		return scrollTape{}, false
	}
	// A tape only scrolls forward: every keyframe must introduce the next
	// state in order, so a state that recurs later rules the tape out.
	for i, frame := range keyframes {
		if frame.state != min(i, len(states)-1) {
			return scrollTape{}, false
		}
	}
	full := make([][]ir.Row, len(states))
	for stateIndex, state := range states {
		full[stateIndex] = make([]ir.Row, band.height)
		for y := range band.height {
			row := rowAt(state, y)
			if !buildVisualRow(row, band.width, colors).supported {
				return scrollTape{}, false
			}
			row.Runs = slices.Clone(row.Runs)
			full[stateIndex][y] = row
		}
	}
	offsets := make([]int, len(states))
	tapeRows := cloneTapeRows(full[0], 0)
	for state := 1; state < len(full); state++ {
		shift := upwardShift(full[state-1], full[state])
		if shift == 0 {
			return scrollTape{}, false
		}
		offsets[state] = offsets[state-1] + shift
		tapeRows = append(tapeRows, cloneTapeRows(full[state][band.height-shift:], len(tapeRows))...)
	}
	points := make([]keyframePoint[int], len(keyframes))
	for i, point := range keyframes {
		points[i] = keyframePoint[int]{selector: point.selector, state: offsets[point.state]}
	}
	return scrollTape{rows: tapeRows, offsets: points}, true
}

func upwardShift(previous, current []ir.Row) int {
	for shift := 1; shift < len(previous); shift++ {
		matched := true
		for y := 0; y < len(previous)-shift; y++ {
			before, after := previous[y+shift], current[y]
			before.Y, after.Y = 0, 0
			if semanticRowHash(before) != semanticRowHash(after) || !rowEqual(before, after) {
				matched = false
				break
			}
		}
		if matched {
			return shift
		}
	}
	return 0
}

func cloneTapeRows(rows []ir.Row, startY int) []ir.Row {
	out := make([]ir.Row, len(rows))
	for i, row := range rows {
		out[i] = row
		out[i].Y = startY + i
		out[i].Runs = slices.Clone(row.Runs)
	}
	return out
}
