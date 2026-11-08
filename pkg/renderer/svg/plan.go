package svg

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

type semanticPlan struct {
	duration          time.Duration
	width             int
	height            int
	staticRows        []ir.Row
	layers            []intervalLayer
	content           timeline[[]ir.Row]
	cursor            timeline[ir.Cursor]
	cursorEverVisible bool
	usedColors        []color.ID
}

type renderPlan = semanticPlan

func buildRenderPlan(rec *ir.Recording, showCursor bool) renderPlan {
	options := DefaultOptions()
	return buildRenderPlanWithOptions(rec, showCursor, &options)
}

func buildRenderPlanWithOptions(rec *ir.Recording, showCursor bool, options *Options) renderPlan {
	plan, err := buildSemanticPlan(context.Background(), rec, showCursor, options.MaxFPS, 1)
	if err != nil {
		return renderPlan{}
	}
	return plan
}

func buildSemanticPlan(
	ctx context.Context,
	rec *ir.Recording,
	showCursor bool,
	maxFPS int,
	loopCount int,
) (semanticPlan, error) {
	if err := contextErr(ctx); err != nil {
		return semanticPlan{}, err
	}
	plan := semanticPlan{duration: rec.Duration, width: rec.Width, height: rec.Height}
	content := make([]timelinePoint[[]ir.Row], 0, len(rec.Frames))
	for _, frame := range rec.Frames {
		content = append(content, timelinePoint[[]ir.Row]{time: frame.Time, state: frame.Rows})
	}
	plan.content = normalizeTimeline(rec.Duration, content, rowsEqual)
	if err := contextErr(ctx); err != nil {
		return semanticPlan{}, err
	}
	plan.content = quantizeTimeline(plan.content, maxFPS, rowsEqual)
	if err := contextErr(ctx); err != nil {
		return semanticPlan{}, err
	}
	plan.hoistStaticRows(rec.Height)
	if err := contextErr(ctx); err != nil {
		return semanticPlan{}, err
	}
	plan.hoistStaticCells(rec.Width, rec.Height, rec.Colors)
	if err := contextErr(ctx); err != nil {
		return semanticPlan{}, err
	}
	plan.splitInertGaps(rec.Colors)
	if err := contextErr(ctx); err != nil {
		return semanticPlan{}, err
	}

	if showCursor {
		cursor := make([]timelinePoint[ir.Cursor], 0, len(rec.Frames))
		for _, frame := range rec.Frames {
			cursor = append(cursor, timelinePoint[ir.Cursor]{time: frame.Time, state: frame.Cursor})
		}
		plan.cursor = normalizeTimeline(rec.Duration, cursor, cursorStatesEqual)
		plan.cursor = quantizeTimeline(plan.cursor, maxFPS, cursorStatesEqual)
		plan.refreshCursorVisibility()
	}
	if err := contextErr(ctx); err != nil {
		return semanticPlan{}, err
	}
	plan.pruneZeroDwellCursorEndpoint(loopCount)
	plan.usedColors = usedColorIDs(rec, plan.staticRows, plan.content.points)
	return plan, contextErr(ctx)
}

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (p *renderPlan) pruneZeroDwellCursorEndpoint(loopCount int) {
	if !infiniteLoop(loopCount) || len(p.cursor.points) < 2 {
		return
	}
	last := p.cursor.points[len(p.cursor.points)-1]
	previous := p.cursor.points[len(p.cursor.points)-2]
	if last.time != p.duration || cursorStatesEqual(last.state, previous.state) {
		return
	}

	// A state introduced exactly at 100% has no dwell time before an infinite
	// animation restarts at 0%. Remove it, then normalize to retain an explicit
	// 100% hold of the preceding state for both CSS and SMIL backends.
	p.cursor = normalizeTimeline(p.duration, p.cursor.points[:len(p.cursor.points)-1], cursorStatesEqual)
	p.refreshCursorVisibility()
}

func infiniteLoop(loopCount int) bool {
	return loopCount == 0 || loopCount < -1
}

func (p *renderPlan) refreshCursorVisibility() {
	effective := p.cursor.keyframes(cursorStatesEqual)
	if len(effective) == 0 && len(p.cursor.points) > 0 {
		effective = []keyframePoint[ir.Cursor]{{state: p.cursor.points[len(p.cursor.points)-1].state}}
	}
	p.cursorEverVisible = slices.ContainsFunc(effective, func(point keyframePoint[ir.Cursor]) bool {
		return point.state.Visible
	})
	if !p.cursorEverVisible {
		p.cursor.points = nil
	}
}

func (p *renderPlan) hoistStaticRows(height int) {
	if len(p.content.points) == 0 {
		return
	}
	static := make(map[int]bool, height)
	for y := range height {
		first := rowAt(p.content.points[0].state, y)
		static[y] = true
		for _, point := range p.content.points[1:] {
			if !rowEqual(first, rowAt(point.state, y)) {
				static[y] = false
				break
			}
		}
		if static[y] && len(first.Runs) > 0 {
			p.staticRows = append(p.staticRows, first)
		}
	}
	for i := range p.content.points {
		rows := make([]ir.Row, 0, len(p.content.points[i].state))
		for _, row := range p.content.points[i].state {
			if !static[row.Y] {
				rows = append(rows, row)
			}
		}
		p.content.points[i].state = rows
	}
}

func rowsEqual(a, b []ir.Row) bool {
	return slices.EqualFunc(a, b, rowEqual)
}

func rowEqual(a, b ir.Row) bool {
	return a.Y == b.Y && slices.Equal(a.Runs, b.Runs)
}

func rowAt(rows []ir.Row, y int) ir.Row {
	for _, row := range rows {
		if row.Y == y {
			return row
		}
	}
	return ir.Row{Y: y}
}

func usedColorIDs(rec *ir.Recording, staticRows []ir.Row, points []timelinePoint[[]ir.Row]) []color.ID {
	used := make(map[color.ID]bool)
	visit := func(rows []ir.Row) {
		for _, row := range rows {
			for _, run := range row.Runs {
				if !rec.Colors.IsDefault(run.Attrs.BG) && runEndCol(run) > run.StartCol {
					used[run.Attrs.BG] = true
				}
				if shouldRenderText(run) && !rec.Colors.IsDefault(run.Attrs.FG) {
					used[run.Attrs.FG] = true
				}
			}
		}
	}
	visit(staticRows)
	for _, point := range points {
		visit(point.state)
	}
	ids := make([]color.ID, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
