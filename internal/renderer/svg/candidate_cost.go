package svg

import (
	"fmt"
	"io"
)

type preparedCandidateCost struct {
	finalBytes int64
}

type preparedContentCost struct {
	definitions int64
	styles      int64
	active      int64
	regionBytes int64
}

type candidateCostLedger struct {
	bytes  int64
	minify bool
}

func (l *candidateCostLedger) add(write func(io.Writer)) {
	counter := &countingWriter{collapseNBSP: l.minify}
	write(counter)
	l.bytes += counter.size()
}

func (l *candidateCostLedger) addBytes(bytes int64) { l.bytes += bytes }

func buildPreparedCandidateCost(c *canvas, candidate *preparedCandidate) preparedCandidateCost {
	ledger := candidateCostLedger{minify: c.config.Minify}
	for _, section := range c.documentSections(&candidate.content) {
		if section.bytes != nil {
			ledger.addBytes(*section.bytes)
		} else {
			ledger.add(section.write)
		}
	}
	return preparedCandidateCost{finalBytes: ledger.bytes}
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
	}
}
