package ir

import (
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/progress"
	"github.com/mrmarble/termsvg/pkg/terminal"
	"github.com/mrmarble/termsvg/pkg/theme"
)

// ProcessorConfig holds options for IR generation.
type ProcessorConfig struct {
	Theme         theme.Theme
	IdleTimeLimit time.Duration          // Cap idle time (0 = no cap)
	Speed         float64                // Playback speed multiplier (1.0 = normal)
	Compress      bool                   // Merge events with same timestamp
	ProgressCh    chan<- progress.Update // Channel for progress updates (optional)
}

// Processor transforms an asciicast into IR.
type Processor struct {
	config ProcessorConfig
}

// DefaultProcessorConfig returns sensible defaults.
func DefaultProcessorConfig() ProcessorConfig {
	return ProcessorConfig{
		Theme:         theme.Default(),
		IdleTimeLimit: 0,
		Speed:         1.0,
		Compress:      true,
	}
}

// NewProcessor creates a new IR processor.
func NewProcessor(config ProcessorConfig) *Processor {
	return &Processor{config: config}
}

// Process transforms a Cast into a Recording (the IR).
func (p *Processor) Process(cast *asciicast.Cast) (*Recording, error) {
	// 1. Pre-process the cast (compress, adjust timing)
	events := p.preprocessEvents(cast)
	totalEvents := len(events)

	// Send initial progress
	if p.config.ProgressCh != nil {
		p.config.ProgressCh <- progress.Update{
			Phase:   "IR Processing",
			Current: 0,
			Total:   totalEvents,
		}
	}

	// 2. Initialize terminal emulator
	term := terminal.New(cast.Header.Width, cast.Header.Height)

	// 3. Initialize color catalog with theme defaults
	catalog := color.NewCatalog(p.config.Theme.Foreground, p.config.Theme.Background)

	// 4. Process each event into a frame
	frames := make([]Frame, 0, len(events))
	stats := Stats{}

	var prevTime time.Duration
	for i, event := range events {
		// Write to terminal emulator
		_, _ = term.Write([]byte(event.EventData))

		// Capture frame
		frameTime := floatSecondsToDuration(event.Time)
		frame := p.captureFrame(term, catalog, i, frameTime, frameTime-prevTime, &stats)
		frames = append(frames, frame)

		// Send progress update every 10 events or on last event
		if p.config.ProgressCh != nil && (i%10 == 0 || i == totalEvents-1) {
			p.config.ProgressCh <- progress.Update{
				Phase:   "IR Processing",
				Current: i + 1,
				Total:   totalEvents,
			}
		}

		prevTime = frameTime
	}

	// 5. Deduplicate consecutive identical frames
	frames = deduplicateFrames(frames)

	// 6. Finalize statistics
	stats.TotalFrames = len(frames)
	stats.UniqueColors = catalog.Count()

	// 6. Calculate duration
	var duration time.Duration
	if len(frames) > 0 {
		duration = frames[len(frames)-1].Time
	}

	return &Recording{
		Width:    cast.Header.Width,
		Height:   cast.Header.Height,
		Duration: duration,
		Title:    cast.Header.Title,
		Frames:   frames,
		Colors:   catalog,
		Stats:    stats,
	}, nil
}

// captureFrame extracts the current terminal state into a Frame.
func (p *Processor) captureFrame(
	term *terminal.Emulator,
	catalog *color.Catalog,
	index int,
	absTime, delay time.Duration,
	stats *Stats,
) Frame {
	rows := make([]Row, term.Height())

	for y := 0; y < term.Height(); y++ {
		rows[y] = p.captureRow(term, catalog, y, stats)
	}

	// Capture cursor state
	cursorCol, cursorRow := term.Cursor()
	cursor := Cursor{
		Col:     cursorCol,
		Row:     cursorRow,
		Visible: term.CursorVisible(),
	}

	return Frame{
		Time:   absTime,
		Delay:  delay,
		Index:  index,
		Rows:   rows,
		Cursor: cursor,
	}
}

// captureRow extracts a single row, grouping cells into TextRuns.
func (p *Processor) captureRow(
	term *terminal.Emulator,
	catalog *color.Catalog,
	y int,
	stats *Stats,
) Row {
	runs := make([]TextRun, 0, 8) // Pre-allocate for typical case

	type runBuilder struct {
		chars  []rune
		startX int
		attrs  CellAttrs
	}

	var current *runBuilder

	for x := 0; x < term.Width(); x++ {
		cell := term.Cell(x, y)
		attrs := p.cellToAttrs(cell, catalog, stats)

		// Check if we can extend the current run
		if current != nil && attrsEqual(current.attrs, attrs) {
			current.chars = append(current.chars, cell.Char)
		} else {
			// Finalize the previous run if exists
			if current != nil {
				runs = append(runs, TextRun{
					Text:     string(current.chars),
					StartCol: current.startX,
					Attrs:    current.attrs,
				})
			}
			// Start a new run
			current = &runBuilder{
				chars:  []rune{cell.Char},
				startX: x,
				attrs:  attrs,
			}
		}
	}

	// Don't forget the last run
	if current != nil {
		runs = append(runs, TextRun{
			Text:     string(current.chars),
			StartCol: current.startX,
			Attrs:    current.attrs,
		})
	}

	// Track statistics
	if len(runs) > stats.MaxRunsPerRow {
		stats.MaxRunsPerRow = len(runs)
	}

	return Row{Y: y, Runs: runs}
}

// cellToAttrs converts a terminal cell to IR attributes.
func (p *Processor) cellToAttrs(
	cell terminal.Cell,
	catalog *color.Catalog,
	stats *Stats,
) CellAttrs {
	// Register colors and get IDs
	fgID := catalog.Register(cell.Foreground, p.config.Theme.Palette)
	bgID := catalog.Register(cell.Background, p.config.Theme.Palette)

	// Track attribute usage
	if cell.Bold {
		stats.HasBold = true
	}
	if cell.Italic {
		stats.HasItalic = true
	}
	if cell.Underline {
		stats.HasUnderline = true
	}
	if cell.Dim {
		stats.HasDim = true
	}
	if cell.Foreground.Type == color.TrueColor || cell.Background.Type == color.TrueColor {
		stats.HasTrueColor = true
	}

	return CellAttrs{
		FG:        fgID,
		BG:        bgID,
		Bold:      cell.Bold,
		Italic:    cell.Italic,
		Underline: cell.Underline,
		Dim:       cell.Dim,
	}
}

// preprocessEvents applies timing adjustments and compression.
func (p *Processor) preprocessEvents(cast *asciicast.Cast) []asciicast.Event {
	// Work with a copy to avoid mutating input
	events := make([]asciicast.Event, len(cast.Events))
	copy(events, cast.Events)

	// Apply speed adjustment
	if p.config.Speed != 1.0 && p.config.Speed > 0 {
		for i := range events {
			events[i].Time /= p.config.Speed
		}
	}

	// Cap idle time (requires conversion to relative, cap, convert back)
	if p.config.IdleTimeLimit > 0 {
		limit := p.config.IdleTimeLimit.Seconds()
		prev := 0.0
		for i := range events {
			delay := events[i].Time - prev
			if delay > limit {
				// Reduce by the excess
				reduction := delay - limit
				// Shift this and all subsequent events
				for j := i; j < len(events); j++ {
					events[j].Time -= reduction
				}
			}
			prev = events[i].Time
		}
	}

	// Compress events with same timestamp
	if p.config.Compress {
		compressed := make([]asciicast.Event, 0, len(events))
		for i, event := range events {
			if i == 0 {
				compressed = append(compressed, event)
				continue
			}
			last := &compressed[len(compressed)-1]
			if event.Time == last.Time {
				last.EventData += event.EventData
			} else {
				compressed = append(compressed, event)
			}
		}
		events = compressed
	}

	return events
}

// attrsEqual compares two CellAttrs for equality.
func attrsEqual(a, b CellAttrs) bool {
	return a.FG == b.FG &&
		a.BG == b.BG &&
		a.Bold == b.Bold &&
		a.Italic == b.Italic &&
		a.Underline == b.Underline &&
		a.Dim == b.Dim
}

func floatSecondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// deduplicateFrames removes consecutive identical frames and consolidates their delays.
// This optimizes the recording by eliminating redundant frames.
func deduplicateFrames(frames []Frame) []Frame {
	if len(frames) <= 1 {
		return frames
	}

	deduped := make([]Frame, 0, len(frames))
	var prevFrame *Frame

	for i, frame := range frames {
		if i == 0 {
			// First frame always kept
			deduped = append(deduped, frame)
			prevFrame = &deduped[len(deduped)-1]
			continue
		}

		// Check if frame is identical to previous
		if framesEqual(prevFrame, &frame) {
			// Duplicate: add delay to previous frame
			prevFrame.Delay += frame.Delay
			// Update absolute time to match the duplicate
			prevFrame.Time = frame.Time
		} else {
			// New unique frame: add it
			deduped = append(deduped, frame)
			prevFrame = &deduped[len(deduped)-1]
		}
	}

	// Renumber frame indices to be sequential
	for i := range deduped {
		deduped[i].Index = i
	}

	return deduped
}

// framesEqual compares two frames for equality (content only, not timing).
func framesEqual(a, b *Frame) bool {
	// Compare cursor state
	if a.Cursor != b.Cursor {
		return false
	}

	// Compare row count
	if len(a.Rows) != len(b.Rows) {
		return false
	}

	// Compare each row
	for i := range a.Rows {
		if !rowsEqual(&a.Rows[i], &b.Rows[i]) {
			return false
		}
	}

	return true
}

// rowsEqual compares two rows for equality.
func rowsEqual(a, b *Row) bool {
	if a.Y != b.Y {
		return false
	}

	if len(a.Runs) != len(b.Runs) {
		return false
	}

	for i := range a.Runs {
		if !textRunsEqual(&a.Runs[i], &b.Runs[i]) {
			return false
		}
	}

	return true
}

// textRunsEqual compares two text runs for equality.
func textRunsEqual(a, b *TextRun) bool {
	return a.Text == b.Text &&
		a.StartCol == b.StartCol &&
		a.Attrs == b.Attrs
}
