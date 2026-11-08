package svg

import (
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
)

func TestBuildRenderPlanQuantizesContentAndCursorIndependently(t *testing.T) {
	row := func(text string) []ir.Row { return []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: text}}}} }
	rec := &ir.Recording{Height: 1, Duration: time.Second, Frames: []ir.Frame{
		{Time: 0, Rows: row("a"), Cursor: ir.Cursor{Visible: true}},
		{Time: 100 * time.Millisecond, Rows: row("b"), Cursor: ir.Cursor{Visible: true}},
		{Time: 400 * time.Millisecond, Rows: row("c"), Cursor: ir.Cursor{Col: 1, Visible: true}},
		{Time: 600 * time.Millisecond, Rows: row("d"), Cursor: ir.Cursor{Col: 1, Visible: true}},
		{Time: 900 * time.Millisecond, Rows: row("e"), Cursor: ir.Cursor{Col: 2, Visible: true}},
	}}
	options := DefaultOptions()
	options.MaxFPS = 2
	plan := buildRenderPlanWithOptions(rec, true, &options)

	if len(plan.content.points) != 3 || plan.content.points[1].time != 500*time.Millisecond ||
		plan.content.points[1].state[0].Runs[0].Text != "c" {
		t.Fatalf("content timeline = %#v", plan.content.points)
	}
	if len(plan.cursor.points) != 3 || plan.cursor.points[1].time != 500*time.Millisecond ||
		plan.cursor.points[1].state.Col != 1 {
		t.Fatalf("cursor timeline = %#v", plan.cursor.points)
	}
	if plan.duration != time.Second || plan.content.duration != time.Second || plan.cursor.duration != time.Second {
		t.Fatalf("duration changed: plan=%v content=%v cursor=%v", plan.duration, plan.content.duration, plan.cursor.duration)
	}
}
