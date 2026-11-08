package svg

import (
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
)

func TestBuildRowBandsUsesLocalHorizontalBounds(t *testing.T) {
	plan := renderPlan{
		duration: time.Second,
		content: timeline[[]ir.Row]{duration: time.Second, points: []timelinePoint[[]ir.Row]{
			{time: 0, state: []ir.Row{{Y: 2, Runs: []ir.TextRun{{Text: "1", StartCol: 10, EndCol: 11}}}}},
			{time: time.Second, state: []ir.Row{{Y: 2, Runs: []ir.TextRun{{Text: "2", StartCol: 10, EndCol: 11}}}}},
		}},
	}

	bands := buildRowBands(&plan, 78, 17)
	if run := plan.content.points[0].state[0].Runs[0]; run.StartCol != 10 || run.EndCol != 11 {
		t.Fatalf("source run was mutated: %+v", run)
	}

	if len(bands) != 1 {
		t.Fatalf("bands = %d; want 1", len(bands))
	}
	band := bands[0]
	if band.x != 9 || band.width != 3 || band.y != 2 || band.height != 1 {
		t.Fatalf("band bounds = x:%d width:%d y:%d height:%d; want 9,3,2,1",
			band.x, band.width, band.y, band.height)
	}
	for i, point := range band.content.points {
		run := point.state[0].Runs[0]
		if run.StartCol != 1 || run.EndCol != 2 {
			t.Fatalf("state %d run extent = [%d,%d); want [1,2)", i, run.StartCol, run.EndCol)
		}
	}
}

func TestBuildRowBandsClampsOverhangAtTerminalEdges(t *testing.T) {
	plan := renderPlan{
		duration: time.Second,
		content: timeline[[]ir.Row]{duration: time.Second, points: []timelinePoint[[]ir.Row]{
			{time: 0, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "a", StartCol: 0, EndCol: 1}}}}},
			{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "b", StartCol: 0, EndCol: 1}}}}},
		}},
	}

	band := buildRowBands(&plan, 4, 1)[0]
	if band.x != 0 || band.width != 2 {
		t.Fatalf("left-edge band = x:%d width:%d; want 0,2", band.x, band.width)
	}
}

func TestBandTimelineSignatureIncludesLocalWidth(t *testing.T) {
	frames := []keyframePoint[int]{{selector: "0%", state: 0}, {selector: "100%", state: 1}}
	if keyframeSignature(frames, 3) == keyframeSignature(frames, 4) {
		t.Fatal("bands with different local widths shared a keyframe signature")
	}
}
