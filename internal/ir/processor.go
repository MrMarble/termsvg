package ir

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/internal/asciicast"
	"github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/terminal"
	"github.com/mrmarble/termsvg/internal/theme"
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

// canvasSize is the largest terminal size seen in a recording. Renderers draw
// every frame on a canvas of this size so a terminal that grows is not clipped.
type canvasSize struct{ cols, rows int }

// maxTerminalSize bounds a terminal dimension, so a corrupt header or resize
// event cannot make the emulator allocate an absurd screen.
const maxTerminalSize = 4096

// DefaultProcessorConfig returns sensible defaults.
func DefaultProcessorConfig() *ProcessorConfig {
	return &ProcessorConfig{
		Theme:         theme.Default(),
		IdleTimeLimit: 0,
		Speed:         1.0,
		Compress:      true,
	}
}

// NewProcessor creates a new IR processor.
func NewProcessor(config *ProcessorConfig) *Processor {
	return &Processor{config: *config}
}

// sendProgress reports IR processing progress when a progress channel is set.
func (p *Processor) sendProgress(current, total int) {
	if p.config.ProgressCh != nil {
		p.config.ProgressCh <- progress.Update{Phase: "IR Processing", Current: current, Total: total}
	}
}

func (p *Processor) Process(cast *asciicast.Cast) (*Recording, error) {
	if err := validateTerminalSize(cast.Header.Width, cast.Header.Height); err != nil {
		return nil, fmt.Errorf("invalid terminal size %dx%d: %w", cast.Header.Width, cast.Header.Height, err)
	}

	events := p.preprocessEvents(cast)
	if !hasOutput(events) {
		return nil, errors.New("recording has no output events")
	}
	p.sendProgress(0, len(events))

	term := terminal.New(cast.Header.Width, cast.Header.Height)
	catalog := color.NewCatalog(p.config.Theme.Foreground, p.config.Theme.Background)

	stats := Stats{}
	canvas := canvasSize{cols: cast.Header.Width, rows: cast.Header.Height}
	frames := p.captureFrames(events, term, catalog, &canvas, &stats)

	for i := range frames {
		frames[i].Rows = cleanRows(frames[i].Rows, catalog)
	}

	// The recording ends with its last event, even when that event changed
	// nothing visible and is merged into the previous frame below.
	var duration time.Duration
	if len(frames) > 0 {
		duration = frames[len(frames)-1].Time
	}

	frames = deduplicateFrames(frames)
	assignDisplayDelays(frames, duration)
	stats.TotalFrames = len(frames)
	stats.UniqueColors = len(catalog.Colors())

	return &Recording{
		Width:    canvas.cols,
		Height:   canvas.rows,
		Duration: duration,
		Title:    cast.Header.Title,
		Frames:   frames,
		Colors:   catalog,
		Stats:    stats,
	}, nil
}

// captureFrames feeds the events to the emulator in order and captures a frame
// after each one. Output is written to the terminal; a resize changes its size.
func (p *Processor) captureFrames(
	events []asciicast.Event,
	term *terminal.Emulator,
	catalog *color.Catalog,
	canvas *canvasSize,
	stats *Stats,
) []Frame {
	frames := make([]Frame, 0, len(events))
	for i, event := range events {
		if event.EventType == asciicast.Resize {
			cols, rows, ok := parseResize(event.EventData)
			if !ok || (cols == term.Width() && rows == term.Height()) {
				continue // nothing changes on screen, so there is no frame to capture
			}
			term.Resize(cols, rows)
			canvas.cols, canvas.rows = max(canvas.cols, cols), max(canvas.rows, rows)
		} else {
			_, _ = term.Write([]byte(event.EventData))
		}

		frames = append(frames, p.captureFrame(term, catalog, i, floatSecondsToDuration(event.Time), stats))

		if i%10 == 0 {
			p.sendProgress(i+1, len(events))
		}
	}
	p.sendProgress(len(events), len(events))

	return frames
}

func validateTerminalSize(cols, rows int) error {
	if cols < 1 || rows < 1 || cols > maxTerminalSize || rows > maxTerminalSize {
		return fmt.Errorf("columns and rows must be between 1 and %d", maxTerminalSize)
	}

	return nil
}

// parseResize reads a resize event's "COLSxROWS" data. Malformed or out of
// range sizes report false and are ignored.
func parseResize(data string) (cols, rows int, ok bool) {
	colsText, rowsText, found := strings.Cut(strings.TrimSpace(data), "x")
	if !found {
		return 0, 0, false
	}
	cols, colsErr := strconv.Atoi(colsText)
	rows, rowsErr := strconv.Atoi(rowsText)
	if colsErr != nil || rowsErr != nil || validateTerminalSize(cols, rows) != nil {
		return 0, 0, false
	}

	return cols, rows, true
}

func hasOutput(events []asciicast.Event) bool {
	return slices.ContainsFunc(events, func(e asciicast.Event) bool { return e.EventType == asciicast.Output })
}

// captureFrame extracts the current terminal state into a Frame.
func (p *Processor) captureFrame(
	term *terminal.Emulator,
	catalog *color.Catalog,
	index int,
	absTime time.Duration,
	stats *Stats,
) Frame {
	rows := make([]Row, term.Height())

	for y := 0; y < term.Height(); y++ {
		rows[y] = p.captureRow(term, catalog, y, stats)
	}

	cursorCol, cursorRow := term.Cursor()
	cursor := Cursor{
		Col:     cursorCol,
		Row:     cursorRow,
		Visible: term.CursorVisible(),
	}

	return Frame{
		Time:   absTime,
		Index:  index,
		Rows:   rows,
		Cursor: cursor,
	}
}

// captureRow extracts a row, grouping consecutive cells with the same attributes into runs.
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

		if current != nil && current.attrs == attrs {
			current.chars = append(current.chars, cell.Char)
		} else {
			if current != nil {
				runs = append(runs, TextRun{
					Text:     string(current.chars),
					StartCol: current.startX,
					EndCol:   x,
					Attrs:    current.attrs,
				})
			}
			current = &runBuilder{
				chars:  []rune{cell.Char},
				startX: x,
				attrs:  attrs,
			}
		}
	}

	if current != nil {
		runs = append(runs, TextRun{
			Text:     string(current.chars),
			StartCol: current.startX,
			EndCol:   term.Width(),
			Attrs:    current.attrs,
		})
	}

	return Row{Y: y, Runs: runs}
}

func (p *Processor) cellToAttrs(
	cell terminal.Cell,
	catalog *color.Catalog,
	stats *Stats,
) CellAttrs {
	fgID := catalog.Register(cell.Foreground, &p.config.Theme.Palette)
	bgID := catalog.Register(cell.Background, &p.config.Theme.Palette)

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
	// Only output and resize events affect the screen: input, markers, exit
	// statuses and unknown codes carry data that is not terminal output. The
	// events are copied so the cast is not mutated.
	events := make([]asciicast.Event, 0, len(cast.Events))
	for _, event := range cast.Events {
		if event.EventType == asciicast.Output || event.EventType == asciicast.Resize {
			events = append(events, event)
		}
	}

	if p.config.Speed != 1.0 && p.config.Speed > 0 {
		for i := range events {
			events[i].Time /= p.config.Speed
		}
	}

	// Keep the legacy O(n²) update order to preserve exact float64 rounding.
	// Replace it only with a bit-exact linear algorithm if large casts become slow.
	if p.config.IdleTimeLimit > 0 {
		limit := p.config.IdleTimeLimit.Seconds()
		prev := 0.0
		for i := range events {
			delay := events[i].Time - prev
			if delay > limit {
				reduction := delay - limit
				for j := i; j < len(events); j++ {
					events[j].Time -= reduction
				}
			}
			prev = events[i].Time
		}
	}

	if p.config.Compress {
		compressed := make([]asciicast.Event, 0, len(events))
		for i, event := range events {
			if i == 0 {
				compressed = append(compressed, event)
				continue
			}
			last := &compressed[len(compressed)-1]
			// Only output events at the same instant can share a frame; a resize
			// carries "COLSxROWS", not terminal output.
			if event.Time == last.Time && event.EventType == asciicast.Output && last.EventType == asciicast.Output {
				last.EventData += event.EventData
			} else {
				compressed = append(compressed, event)
			}
		}
		events = compressed
	}

	return events
}

func floatSecondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// cleanRows strips empty rows and trims trailing whitespace from runs.
func cleanRows(rows []Row, catalog *color.Catalog) []Row {
	cleaned := make([]Row, 0, len(rows))
	for _, row := range rows {
		row.Runs = cleanRuns(row.Runs, catalog)
		if len(row.Runs) > 0 {
			cleaned = append(cleaned, row)
		}
	}
	return cleaned
}

// cleanRuns removes invisible runs and trims trailing whitespace.
func cleanRuns(runs []TextRun, catalog *color.Catalog) []TextRun {
	cleaned := make([]TextRun, 0, len(runs))
	for _, run := range runs {
		if run.Text == "" {
			continue
		}
		if strings.TrimSpace(run.Text) == "" && catalog.IsDefault(run.Attrs.BG) && !run.Attrs.Underline {
			continue
		}
		cleaned = append(cleaned, run)
	}

	// Trim trailing whitespace from the last run (if default BG)
	if n := len(cleaned); n > 0 {
		last := &cleaned[n-1]
		if catalog.IsDefault(last.Attrs.BG) && !last.Attrs.Underline {
			trimmed := strings.TrimRight(last.Text, " ")
			last.EndCol -= len(last.Text) - len(trimmed)
			last.Text = trimmed
			if last.Text == "" {
				cleaned = cleaned[:n-1]
			}
		}
	}

	return cleaned
}

func deduplicateFrames(frames []Frame) []Frame {
	if len(frames) <= 1 {
		return frames
	}

	deduped := make([]Frame, 0, len(frames))
	for i := range frames {
		// A frame identical to the previous one adds nothing: the previous frame
		// simply stays on screen. It keeps the time it first appeared at.
		if len(deduped) > 0 && framesEqual(&deduped[len(deduped)-1], &frames[i]) {
			continue
		}
		deduped = append(deduped, frames[i])
	}

	for i := range deduped {
		deduped[i].Index = i
	}

	return deduped
}

// assignDisplayDelays sets each frame's Delay to how long it stays on screen:
// until the next frame appears or, for the last frame, until the recording ends.
func assignDisplayDelays(frames []Frame, duration time.Duration) {
	for i := range frames {
		end := duration
		if i+1 < len(frames) {
			end = frames[i+1].Time
		}
		frames[i].Delay = max(0, end-frames[i].Time)
	}
}

// framesEqual compares visible frame content, including the cursor, not timing.
func framesEqual(a, b *Frame) bool {
	if a.Cursor != b.Cursor {
		return false
	}

	if len(a.Rows) != len(b.Rows) {
		return false
	}

	for i := range a.Rows {
		if !rowsEqual(&a.Rows[i], &b.Rows[i]) {
			return false
		}
	}

	return true
}

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

func textRunsEqual(a, b *TextRun) bool {
	return a.Text == b.Text &&
		a.StartCol == b.StartCol &&
		a.EndCol == b.EndCol &&
		a.Attrs == b.Attrs
}
