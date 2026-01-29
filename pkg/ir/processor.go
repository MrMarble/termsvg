package ir

import (
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/terminal"
	"github.com/mrmarble/termsvg/pkg/theme"
)

// ProcessorConfig holds options for IR generation.
type ProcessorConfig struct {
	Theme         theme.Theme
	IdleTimeLimit time.Duration // Cap idle time (0 = no cap)
	Speed         float64       // Playback speed multiplier (1.0 = normal)
	Compress      bool          // Merge events with same timestamp
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

// Processor transforms an asciicast into IR.
type Processor struct {
	config ProcessorConfig
}

// NewProcessor creates a new IR processor.
func NewProcessor(config ProcessorConfig) *Processor {
	return &Processor{config: config}
}

// Process transforms a Cast into a Recording (the IR).
func (p *Processor) Process(cast *asciicast.Cast) (*Recording, error) {
	// 1. Pre-process the cast (compress, adjust timing)
	events := p.preprocessEvents(cast)

	// 2. Initialize terminal emulator
	term := terminal.New(cast.Header.Width, cast.Header.Height)

	// 3. Initialize color catalog with theme defaults
	catalog := color.NewColorCatalog(p.config.Theme.Foreground, p.config.Theme.Background)

	// 4. Process each event into a frame
	frames := make([]Frame, 0, len(events))
	stats := Stats{}

	var prevTime time.Duration
	for i, event := range events {
		// Write to terminal emulator
		term.Write([]byte(event.EventData))

		// Capture frame
		frameTime := floatSecondsToDuration(event.Time)
		frame := p.captureFrame(term, catalog, i, frameTime, frameTime-prevTime, &stats)
		frames = append(frames, frame)

		prevTime = frameTime
	}

	// 5. Finalize statistics
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
	catalog *color.ColorCatalog,
	index int,
	absTime, delay time.Duration,
	stats *Stats,
) Frame {
	rows := make([]Row, term.Height())

	for y := 0; y < term.Height(); y++ {
		rows[y] = p.captureRow(term, catalog, y, stats)
	}

	return Frame{
		Time:  absTime,
		Delay: delay,
		Index: index,
		Rows:  rows,
	}
}

// captureRow extracts a single row, grouping cells into TextRuns.
func (p *Processor) captureRow(
	term *terminal.Emulator,
	catalog *color.ColorCatalog,
	y int,
	stats *Stats,
) Row {
	runs := make([]TextRun, 0, 8) // Pre-allocate for typical case

	var currentRun *TextRun

	for x := 0; x < term.Width(); x++ {
		cell := term.Cell(x, y)
		attrs := p.cellToAttrs(cell, catalog, stats)

		// Check if we can extend the current run
		if currentRun != nil && attrsEqual(currentRun.Attrs, attrs) {
			currentRun.Text += string(cell.Char)
		} else {
			// Start a new run
			if currentRun != nil {
				runs = append(runs, *currentRun)
			}
			currentRun = &TextRun{
				Text:     string(cell.Char),
				StartCol: x,
				Attrs:    attrs,
			}
		}
	}

	// Don't forget the last run
	if currentRun != nil {
		runs = append(runs, *currentRun)
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
	catalog *color.ColorCatalog,
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
