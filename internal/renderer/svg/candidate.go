package svg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/renderer"
	"github.com/mrmarble/termsvg/internal/svgoutput"
)

type preparedCandidate struct {
	plan    *semanticPlan
	options Options
	content preparedContent
	metrics CandidateMetrics
	cost    preparedCandidateCost
	style   stylePlan
}

type countingWriter struct {
	bytes        int64
	collapseNBSP bool
	pendingC2    bool
}

// prepareBestCandidate prepares the configured candidate for every plan
// variant and keeps the one the objective prefers. Interval-static layers are
// chosen by an estimate, so the plan without them is measured as well.
func (r *Renderer) prepareBestCandidate(ctx context.Context, rec *ir.Recording) (*preparedCandidate, error) {
	if len(rec.Frames) == 0 {
		return nil, fmt.Errorf("recording has no frames")
	}
	if err := r.options.Validate(); err != nil {
		return nil, err
	}
	plan, err := r.buildSemanticPlan(ctx, rec)
	if err != nil {
		return nil, err
	}
	layered, err := r.selectIntervalLayers(ctx, rec, &plan)
	if err != nil {
		return nil, err
	}
	plans := []semanticPlan{plan}
	if len(layered.layers) > 0 {
		plans = append(plans, layered)
	}
	candidates := make([]*preparedCandidate, 0, len(plans))
	for i := range plans {
		candidate, err := r.prepareSelectedCandidate(ctx, rec, &plans[i])
		if err != nil {
			return nil, err
		}
		if candidate.metrics.FinalBytes == 0 {
			if err := r.measureCandidate(ctx, rec, candidate); err != nil {
				return nil, err
			}
		}
		candidates = append(candidates, candidate)
	}
	selected := selectPreparedCandidate(r.options.AutoObjective, candidates...)
	if r.config.Debug && len(candidates) > 1 {
		for _, candidate := range candidates {
			log.Printf("[SVG] plan variant layers=%d bytes=%d selected=%t",
				len(candidate.plan.layers), candidate.metrics.FinalBytes, candidate == selected)
		}
	}
	return selected, nil
}

// measureBest renders the configured candidate to a sink and reports its
// exact serialized size and structural cost.
func (r *Renderer) measureBest(ctx context.Context, rec *ir.Recording) (CandidateMetrics, error) {
	candidate, err := r.prepareBestCandidate(ctx, rec)
	if err != nil {
		return CandidateMetrics{}, err
	}
	return candidate.metrics, nil
}

func (r *Renderer) prepareSelectedCandidate(
	ctx context.Context,
	rec *ir.Recording,
	plan *semanticPlan,
) (*preparedCandidate, error) {
	if r.options.Layout != LayoutAuto {
		return prepareCandidate(ctx, rec, plan, &r.config, &r.options)
	}
	options := r.options
	options.Layout = LayoutFrames
	frames, err := prepareCandidate(ctx, rec, plan, &r.config, &options)
	if err != nil {
		return nil, err
	}
	if err := r.measureCandidate(ctx, rec, frames); err != nil {
		return nil, err
	}
	options.Layout = LayoutBands
	bands, err := prepareCandidate(ctx, rec, plan, &r.config, &options)
	if err != nil {
		return nil, err
	}
	if err := r.measureCandidate(ctx, rec, bands); err != nil {
		return nil, err
	}
	options.Layout = LayoutRegions
	regions, err := prepareCandidate(ctx, rec, plan, &r.config, &options)
	if err != nil {
		return nil, err
	}
	if err := r.measureCandidate(ctx, rec, regions); err != nil {
		return nil, err
	}
	selected := selectPreparedCandidate(r.options.AutoObjective, frames, bands, regions)
	r.logAutoCandidates(selected, frames, bands, regions)
	return selected, nil
}

func selectPreparedCandidate(objective AutoObjective, candidates ...*preparedCandidate) *preparedCandidate {
	selected := candidates[0]
	for _, candidate := range candidates[1:] {
		if objective == AutoObjectiveRuntime && runtimeCandidateLess(&candidate.metrics, &selected.metrics) ||
			objective != AutoObjectiveRuntime && candidate.metrics.FinalBytes < selected.metrics.FinalBytes {
			selected = candidate
		}
	}
	return selected
}

func runtimeCandidateLess(left, right *CandidateMetrics) bool {
	comparisons := [...]struct{ left, right uint64 }{
		{left.PeakLiveNodeEstimate, right.PeakLiveNodeEstimate},
		{left.DurationWeightedInstantiatedNodeNanos, right.DurationWeightedInstantiatedNodeNanos},
		{left.UseTargetSwitches, right.UseTargetSwitches},
	}
	for _, comparison := range comparisons {
		if comparison.left != comparison.right {
			return comparison.left < comparison.right
		}
	}
	integerComparisons := [...]struct{ left, right int64 }{
		{int64(left.AnimationNodes), int64(right.AnimationNodes)},
		{int64(left.AnimatedElements), int64(right.AnimatedElements)},
		{int64(left.LocalViewportCount), int64(right.LocalViewportCount)},
		{left.MaxViewportArea, right.MaxViewportArea},
		{left.MaxTranslatedArea, right.MaxTranslatedArea},
		{left.FinalBytes, right.FinalBytes},
	}
	for _, comparison := range integerComparisons {
		if comparison.left != comparison.right {
			return comparison.left < comparison.right
		}
	}
	return false
}

func (r *Renderer) logAutoCandidates(selected *preparedCandidate, candidates ...*preparedCandidate) {
	if !r.config.Debug {
		return
	}
	for _, candidate := range candidates {
		metrics := candidate.metrics
		log.Printf("[SVG] auto candidate layout=%s bytes=%d source_nodes=%d definition_nodes=%d "+
			"peak_instantiated_nodes=%d animation_nodes=%d viewport_count=%d viewport_area=%d translated_area=%d "+
			"objective=%s selected=%s",
			candidate.options.Layout, metrics.FinalBytes, metrics.SourceActiveNodes, metrics.SourceDefinitionNodes,
			metrics.PeakInstantiatedNodes, metrics.AnimationNodes, metrics.LocalViewportCount, metrics.MaxViewportArea,
			metrics.MaxTranslatedArea,
			r.options.AutoObjective, selected.options.Layout)
	}
}

func prepareCandidate(
	ctx context.Context,
	rec *ir.Recording,
	plan *semanticPlan,
	config *renderer.Config,
	options *Options,
) (*preparedCandidate, error) {
	if options.Style == StyleAuto {
		legacy, _, err := prepareStyleCandidate(ctx, rec, plan, config, options, styleLegacy)
		if err != nil {
			return nil, err
		}
		selected := legacy
		for _, scheme := range []styleScheme{styleAtomic, styleComposite, styleInheritedAtomic, styleInheritedComposite} {
			candidate, stable, err := prepareStyleCandidate(ctx, rec, plan, config, options, scheme)
			if err != nil {
				return nil, err
			}
			if stable && candidate.cost.finalBytes < selected.cost.finalBytes {
				selected = candidate
			}
		}
		attachStyleCostLedger(rec, plan, config, selected)
		return selected, nil
	}
	candidate, _, err := prepareStyleCandidate(ctx, rec, plan, config, options, styleLegacy)
	return candidate, err
}

func prepareStyleCandidate(
	ctx context.Context,
	rec *ir.Recording,
	plan *semanticPlan,
	config *renderer.Config,
	options *Options,
	scheme styleScheme,
) (*preparedCandidate, bool, error) {
	c := newCanvas(rec, plan, config, options)
	if options.Style == StyleAuto {
		c.style = c.styles().legacyStylePlan()
	}
	if scheme == styleLegacy {
		content, err := c.prepareContentContext(ctx)
		if err != nil {
			return nil, false, err
		}
		if options.Style == StyleAuto {
			c.style.occurrences = c.styles().countPaintOccurrences(&content)
		}
		candidate, err := finishPreparedCandidate(c, plan, options, &content)
		return candidate, true, err
	}

	empty := paintOccurrences{texts: map[textStyleKey]int{}, backgrounds: map[color.ID]int{}}
	style := c.styles().buildStylePlan(scheme, empty)
	// Bound the naming/interning fixed-point search to four passes. Schemes that
	// do not stabilize are skipped so another stable candidate can win.
	for range 4 {
		c.style = style
		content, err := c.prepareContentContext(ctx)
		if err != nil {
			return nil, false, err
		}
		next := c.styles().buildStylePlan(scheme, c.styles().countPaintOccurrences(&content))
		if stylePlanEqual(&style, &next) {
			c.style = next
			candidate, err := finishPreparedCandidate(c, plan, options, &content)
			return candidate, true, err
		}
		style = next
	}
	return nil, false, nil
}

func finishPreparedCandidate(
	c *canvas,
	plan *semanticPlan,
	options *Options,
	content *preparedContent,
) (*preparedCandidate, error) {
	candidate := &preparedCandidate{plan: plan, options: *options, content: *content, style: c.style}
	addPreparedMetrics(&candidate.metrics, content, options, c.contentWidth(), c.contentHeight())
	if err := addStructuralMetrics(&candidate.metrics, c, content); err != nil {
		return nil, err
	}
	candidate.cost = buildPreparedCandidateCost(c, candidate)
	return candidate, nil
}

func attachStyleCostLedger(
	rec *ir.Recording,
	plan *semanticPlan,
	config *renderer.Config,
	candidate *preparedCandidate,
) {
	c := newCanvas(rec, plan, config, &candidate.options)
	c.style = candidate.style
	candidate.style.cost = c.buildStyleCostLedger()
	candidate.style.styleBytes = candidate.style.cost.total
}

func (r *Renderer) measureCandidate(ctx context.Context, rec *ir.Recording, candidate *preparedCandidate) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	candidate.metrics.FinalBytes = candidate.cost.finalBytes
	return nil
}

func writeFinalSVG(w io.Writer, minify bool, render func(io.Writer) error) error {
	if minify {
		return svgoutput.Write(w, render)
	}
	return render(w)
}

func (r *Renderer) serializeCandidate(
	ctx context.Context,
	rec *ir.Recording,
	w io.Writer,
	candidate *preparedCandidate,
) error {
	buf := bufio.NewWriterSize(w, 64*1024)
	c := newCanvas(rec, candidate.plan, &r.config, &candidate.options)
	c.w, c.metrics, c.style = buf, &candidate.metrics, candidate.style
	if err := c.render(ctx, &candidate.content); err != nil {
		return err
	}
	return buf.Flush()
}

func (w *countingWriter) Write(p []byte) (int, error) {
	if !w.collapseNBSP {
		w.bytes += int64(len(p))
		return len(p), nil
	}
	for _, value := range p {
		if w.pendingC2 {
			if value == 0xa0 {
				w.bytes++
				w.pendingC2 = false
				continue
			}
			w.bytes++
			w.pendingC2 = false
		}
		if value == 0xc2 {
			w.pendingC2 = true
		} else {
			w.bytes++
		}
	}
	return len(p), nil
}

func (w *countingWriter) size() int64 {
	if w.pendingC2 {
		return w.bytes + 1
	}
	return w.bytes
}
