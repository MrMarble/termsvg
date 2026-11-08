package svg

import (
	"context"
	"fmt"
	"io"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

type preparedCandidateCost struct {
	finalBytes    int64
	fragmentWalks int
}

type preparedContentCost struct {
	definitions   int64
	styles        int64
	active        int64
	regionBytes   int64
	fragmentWalks int
}

type candidateCostLedger struct {
	bytes         int64
	minify        bool
	fragmentWalks int
}

func (l *candidateCostLedger) add(write func(io.Writer)) {
	counter := &countingWriter{collapseNBSP: l.minify}
	write(counter)
	l.bytes += counter.size()
	l.fragmentWalks++
}

func (l *candidateCostLedger) addBytes(bytes int64) { l.bytes += bytes }

func costPreparedCandidate(
	ctx context.Context,
	_ *ir.Recording,
	_ *renderer.Config,
	candidate *preparedCandidate,
) (int64, error) {
	if err := contextErr(ctx); err != nil {
		return 0, err
	}
	return candidate.cost.finalBytes, nil
}

func costPreparedContent(ctx context.Context, _ *canvas, content *preparedContent) (int64, error) {
	if err := contextErr(ctx); err != nil {
		return 0, err
	}
	return content.cost.regionBytes, nil
}

func buildPreparedCandidateCost(c *canvas, candidate *preparedCandidate) preparedCandidateCost {
	ledger := candidateCostLedger{minify: c.config.Minify}
	ledger.add(func(w io.Writer) { c.w = w; c.writeSVGOpen() })
	if c.config.ShowWindow {
		ledger.add(func(w io.Writer) { c.w = w; c.writeWindow() })
	} else {
		ledger.add(func(w io.Writer) { c.w = w; c.writeBackground() })
	}
	ledger.add(func(w io.Writer) {
		fmt.Fprintf(w, `<defs><clipPath id="clip"><rect width="%s" height="%s"/></clipPath>`,
			c.xmlInt(c.contentWidth()), c.xmlInt(c.contentHeight()))
	})
	ledger.addBytes(candidate.content.cost.definitions)
	ledger.add(func(w io.Writer) { fmt.Fprint(w, `</defs>`) })
	contentY := Padding
	if c.config.ShowWindow {
		contentY = Padding * HeaderSize
	}
	ledger.add(func(w io.Writer) { c.w = w; c.writeContentGroupOpen(contentY) })
	ledger.addBytes(candidate.content.cost.styles)
	for _, row := range c.plan.staticRows {
		row := row
		ledger.add(func(w io.Writer) { c.writeRow(w, row) })
	}
	for index := range c.plan.layers {
		ledger.add(func(w io.Writer) { c.writeLayer(w, index) })
	}
	ledger.addBytes(candidate.content.cost.active)
	ledger.add(func(w io.Writer) { c.w = w; c.writeCursor() })
	ledger.add(func(w io.Writer) { fmt.Fprint(w, `</g></svg>`) })
	return preparedCandidateCost{
		finalBytes:    ledger.bytes,
		fragmentWalks: ledger.fragmentWalks + candidate.content.cost.fragmentWalks,
	}
}

func buildPreparedContentCost(c *canvas, content *preparedContent) preparedContentCost {
	definitions := candidateCostLedger{minify: c.config.Minify}
	for _, row := range content.rowDefs {
		definition := row.definition
		definitions.add(func(w io.Writer) { fmt.Fprint(w, definition) })
	}
	definitions.add(func(w io.Writer) { c.w = w; c.writeStateDefs(content) })
	styles := candidateCostLedger{minify: c.config.Minify}
	styles.add(func(w io.Writer) { c.w = w; c.writeStyles(content) })
	active := candidateCostLedger{minify: c.config.Minify}
	if c.options.usesLocalViewports() {
		for i := range content.bands {
			band := &content.bands[i]
			active.add(func(w io.Writer) { c.w = w; c.writeBand(band) })
		}
	} else {
		active.add(func(w io.Writer) {
			c.w = w
			c.writeFrames(content.frameRows, content.frameKeyframes, content.frameStateIDs)
		})
	}
	regionBytes := int64(len(`<svg><defs>`)+len(`</defs>`)+len(`</svg>`)) + definitions.bytes + styles.bytes + active.bytes
	return preparedContentCost{
		definitions: definitions.bytes, styles: styles.bytes, active: active.bytes, regionBytes: regionBytes,
		fragmentWalks: definitions.fragmentWalks + styles.fragmentWalks + active.fragmentWalks,
	}
}
