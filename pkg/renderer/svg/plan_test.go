package svg

import (
	"reflect"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
)

func TestBuildRenderPlanSplitsContentAndCursorTimelines(t *testing.T) {
	row := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: "same"}}}
	rec := &ir.Recording{
		Height:   1,
		Duration: 3 * time.Second,
		Frames: []ir.Frame{
			{Time: 0, Rows: []ir.Row{row}, Cursor: ir.Cursor{Visible: true}},
			{Time: time.Second, Rows: []ir.Row{row}, Cursor: ir.Cursor{Col: 1, Visible: true}},
			{Time: time.Second, Rows: []ir.Row{row}, Cursor: ir.Cursor{Col: 2, Visible: true}},
			{Time: 2 * time.Second, Rows: []ir.Row{row}, Cursor: ir.Cursor{Col: 2}},
		},
	}

	plan := buildRenderPlan(rec, true)

	if plan.duration != rec.Duration || len(plan.content.points) != 1 {
		t.Fatalf("content plan = duration %v, %d frames", plan.duration, len(plan.content.points))
	}
	wantCursor := []timelinePoint[ir.Cursor]{
		{time: 0, state: ir.Cursor{Visible: true}},
		{time: time.Second, state: ir.Cursor{Col: 2, Visible: true}},
		{time: 2 * time.Second, state: ir.Cursor{Col: 2}},
		{time: 3 * time.Second, state: ir.Cursor{Col: 2}},
	}
	if !plan.cursorEverVisible || !reflect.DeepEqual(plan.cursor.points, wantCursor) {
		t.Fatalf("cursor plan = %#v, want %#v", plan.cursor, wantCursor)
	}
}

func TestBuildRenderPlanHoistsOnlyRowsStaticAcrossMissingStates(t *testing.T) {
	static := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: "static"}}}
	dynamic := ir.Row{Y: 1, Runs: []ir.TextRun{{Text: "dynamic"}}}
	rec := &ir.Recording{Height: 3, Duration: time.Second, Frames: []ir.Frame{
		{Rows: []ir.Row{static}},
		{Time: time.Second, Rows: []ir.Row{static, dynamic}},
	}}
	wantFrames := append([]ir.Frame(nil), rec.Frames...)
	wantFrames[0].Rows = append([]ir.Row(nil), rec.Frames[0].Rows...)
	wantFrames[1].Rows = append([]ir.Row(nil), rec.Frames[1].Rows...)

	plan := buildRenderPlan(rec, false)

	if !reflect.DeepEqual(plan.staticRows, []ir.Row{static}) {
		t.Fatalf("static rows = %#v, want %#v", plan.staticRows, []ir.Row{static})
	}
	if len(plan.content.points) != 2 || len(plan.content.points[0].state) != 0 ||
		!reflect.DeepEqual(plan.content.points[1].state, []ir.Row{dynamic}) {
		t.Fatalf("dynamic frames = %#v", plan.content.points)
	}
	if plan.cursorEverVisible || len(plan.cursor.points) != 0 {
		t.Fatalf("disabled cursor plan = %#v", plan.cursor)
	}
	if !reflect.DeepEqual(rec.Frames, wantFrames) {
		t.Fatal("building the render plan mutated source frames")
	}
}
