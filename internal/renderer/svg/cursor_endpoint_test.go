package svg

import (
	"testing"
	"time"

	"github.com/mrmarble/termsvg/internal/ir"
)

func TestPruneZeroDwellCursorEndpointForInfiniteLoop(t *testing.T) {
	duration := time.Second
	plan := semanticPlan{
		duration: duration,
		cursor: timeline[ir.Cursor]{duration: duration, points: []timelinePoint[ir.Cursor]{
			{time: 0, state: ir.Cursor{Visible: false}},
			{time: duration, state: ir.Cursor{Visible: true}},
		}},
		cursorEverVisible: true,
	}

	plan.pruneZeroDwellCursorEndpoint(0)

	if plan.cursorEverVisible {
		t.Fatal("endpoint-only visible cursor remained visible")
	}
	if len(plan.cursor.points) != 0 {
		t.Fatalf("cursor points = %v; want omitted cursor timeline", plan.cursor.points)
	}
}

func TestPruneZeroDwellCursorEndpointPreservesFiniteLoop(t *testing.T) {
	duration := time.Second
	points := []timelinePoint[ir.Cursor]{
		{time: 0, state: ir.Cursor{Visible: false}},
		{time: duration, state: ir.Cursor{Visible: true}},
	}
	for _, loops := range []loopCount{-1, 1, 3} {
		plan := semanticPlan{
			duration: duration,
			cursor: timeline[ir.Cursor]{
				duration: duration,
				points:   append([]timelinePoint[ir.Cursor](nil), points...),
			},
			cursorEverVisible: true,
		}
		plan.pruneZeroDwellCursorEndpoint(loops)
		if !plan.cursorEverVisible || len(plan.cursor.points) != 2 || !plan.cursor.points[1].state.Visible {
			t.Fatalf("loop %d did not preserve final cursor state: %+v", loops, plan)
		}
	}
}

func TestPruneZeroDwellCursorEndpointPreservesVisibleDwell(t *testing.T) {
	duration := time.Second
	plan := semanticPlan{
		duration: duration,
		cursor: timeline[ir.Cursor]{duration: duration, points: []timelinePoint[ir.Cursor]{
			{time: 0, state: ir.Cursor{Visible: false}},
			{time: duration / 2, state: ir.Cursor{Visible: true}},
			{time: duration, state: ir.Cursor{Visible: true}},
		}},
		cursorEverVisible: true,
	}

	plan.pruneZeroDwellCursorEndpoint(0)

	if !plan.cursorEverVisible || len(plan.cursor.points) != 3 {
		t.Fatalf("visible cursor with dwell time was modified: %+v", plan)
	}
}
