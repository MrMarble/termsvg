// Package ir provides an intermediate representation for terminal recordings.
// It decouples terminal emulation from rendering, allowing multiple output
// formats (SVG, GIF, etc.) to consume the same pre-processed frame data.
package ir

import (
	"time"

	"github.com/mrmarble/termsvg/internal/color"
)

// Recording is a processed terminal recording, the input of every renderer.
type Recording struct {
	Width    int
	Height   int
	Duration time.Duration
	Title    string // only read by tests of the SVG renderer

	Frames []Frame
	Colors *color.Catalog

	Stats Stats
}

// Stats tells renderers which text styles occur, so they can skip the unused ones.
type Stats struct {
	TotalFrames  int // only read by tests of the SVG renderer
	UniqueColors int // only read by tests of the SVG renderer
	HasBold      bool
	HasItalic    bool
	HasUnderline bool
	HasDim       bool
	HasTrueColor bool // only read by tests of the SVG renderer
}

// Cursor represents the cursor state at a point in time.
type Cursor struct {
	Col     int
	Row     int
	Visible bool
}

// Frame represents terminal state at a specific point in time.
type Frame struct {
	// Time is when the frame first appears, as an offset from the start of the
	// recording. Consecutive identical states are one frame, which keeps the
	// time of the first of them.
	Time time.Duration

	// Delay is how long the frame stays on screen: until the next frame
	// appears or, for the last frame, until the recording ends.
	Delay time.Duration

	// Index is the frame number (0-indexed). Only tests of the SVG renderer read it.
	Index int

	// Rows contains the processed row data with text runs
	Rows []Row

	// Cursor holds the cursor position and visibility
	Cursor Cursor
}

// Row represents a single line of terminal output.
type Row struct {
	// Y is the row index (0-indexed)
	Y int

	// Runs are groups of consecutive cells with the same attributes
	Runs []TextRun
}

// TextRun is a group of consecutive characters sharing the same attributes.
// This is a key optimization - instead of per-cell data, cells are grouped.
type TextRun struct {
	// Text is the concatenated characters in this run
	Text string

	// StartCol is the starting column (0-indexed)
	StartCol int

	// EndCol is the exclusive terminal-cell column.
	EndCol int

	// Attrs holds the visual attributes for this run
	Attrs CellAttrs
}

// CellAttrs holds the visual attributes for a cell or run.
type CellAttrs struct {
	// FG and BG are color catalog IDs (not raw colors).
	// Using IDs enables efficient CSS class generation and color deduplication.
	FG color.ID
	BG color.ID

	// Text styling flags
	Bold      bool
	Italic    bool
	Underline bool
	Dim       bool
}
