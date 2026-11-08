package svg

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
)

func experimentalRow(y int, text string) ir.Row {
	if text == "" {
		return ir.Row{Y: y}
	}
	return ir.Row{Y: y, Runs: []ir.TextRun{{Text: text, EndCol: len(text)}}}
}

func TestBuildRowBandsGroupsOnlyAdjacentRowsWithMatchingSchedules(t *testing.T) {
	plan := renderPlan{
		duration: 2 * time.Second,
		content: timeline[[]ir.Row]{
			duration: 2 * time.Second,
			points: []timelinePoint[[]ir.Row]{
				{state: []ir.Row{experimentalRow(0, "a"), experimentalRow(1, "b"), experimentalRow(2, "c")}},
				{time: time.Second, state: []ir.Row{experimentalRow(0, "A"), experimentalRow(1, "B"), experimentalRow(2, "c")}},
				{
					time: 1500 * time.Millisecond,
					state: []ir.Row{
						experimentalRow(0, "A"), experimentalRow(1, "B"), experimentalRow(2, "C"),
					},
				},
				{time: 2 * time.Second, state: []ir.Row{experimentalRow(0, "A"), experimentalRow(1, "B"), experimentalRow(2, "C")}},
			},
		},
	}

	bands := buildRowBands(&plan, 1, 3)
	if len(bands) != 2 {
		t.Fatalf("bands = %#v, want two bands", bands)
	}
	if bands[0].y != 0 || len(bands[0].content.points[0].state) != 2 {
		t.Fatalf("first band = %#v, want adjacent rows 0 and 1", bands[0])
	}
	if got := []int{
		bands[0].content.points[0].state[0].Y,
		bands[0].content.points[0].state[1].Y,
	}; !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("relative row coordinates = %v", got)
	}
	if bands[1].y != 2 || len(bands[1].content.points) != 3 {
		t.Fatalf("second band = %#v, want row 2 on its later schedule", bands[1])
	}
}

func TestBuildRowBandsDoesNotJoinNonAdjacentRowsWithSameSchedule(t *testing.T) {
	plan := renderPlan{
		duration: time.Second,
		content: timeline[[]ir.Row]{duration: time.Second, points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{experimentalRow(0, "a"), experimentalRow(1, "middle"), experimentalRow(2, "x")}},
			{
				time: 500 * time.Millisecond,
				state: []ir.Row{
					experimentalRow(0, "b"), experimentalRow(1, "MIDDLE"), experimentalRow(2, "y"),
				},
			},
			{
				time: 750 * time.Millisecond,
				state: []ir.Row{
					experimentalRow(0, "b"), experimentalRow(1, "middle"), experimentalRow(2, "y"),
				},
			},
			{time: time.Second, state: []ir.Row{experimentalRow(0, "b"), experimentalRow(1, "middle"), experimentalRow(2, "y")}},
		}},
	}

	bands := buildRowBands(&plan, 6, 3)
	if len(bands) != 3 {
		t.Fatalf("bands = %#v, want three separate bands", bands)
	}
	for i, band := range bands {
		if band.y != i {
			t.Fatalf("band %d starts at %d", i, band.y)
		}
	}
}

func TestRowBandsReconstructEveryPlannedContentState(t *testing.T) {
	static := experimentalRow(1, "static")
	rec := &ir.Recording{Width: 6, Height: 4, Duration: 2 * time.Second, Frames: []ir.Frame{
		{Rows: []ir.Row{experimentalRow(0, "a"), static, experimentalRow(3, "x")}},
		{Time: time.Second, Rows: []ir.Row{experimentalRow(0, "b"), static}},
		{Time: 2 * time.Second, Rows: []ir.Row{experimentalRow(0, "b"), static, experimentalRow(3, "z")}},
	}}
	plan := buildRenderPlan(rec, false)
	bands := buildRowBands(&plan, rec.Width, rec.Height)

	for _, point := range plan.content.points {
		got := append([]ir.Row(nil), plan.staticRows...)
		for _, band := range bands {
			rows := bandStateAt(band, point.time)
			for _, row := range rows {
				row.Y += band.y
				got = append(got, row)
			}
		}
		expected := append(append([]ir.Row(nil), plan.staticRows...), point.state...)
		slices.SortFunc(got, func(a, b ir.Row) int { return a.Y - b.Y })
		slices.SortFunc(expected, func(a, b ir.Row) int { return a.Y - b.Y })
		if !rowsEqual(got, expected) {
			t.Fatalf("state at %v = %#v, want %#v", point.time, got, expected)
		}
	}
}

func bandStateAt(band rowBand, at time.Duration) []ir.Row {
	state := band.content.points[0].state
	for _, point := range band.content.points[1:] {
		if point.time > at {
			break
		}
		state = point.state
	}
	return state
}
