// Package ir provides an intermediate representation for terminal recordings.
// It decouples terminal emulation from rendering, allowing multiple output
// formats (SVG, GIF, etc.) to consume the same pre-processed frame data.
package ir

import (
	"time"

	"github.com/mrmarble/termsvg/pkg/color"
)

// Recording represents the complete intermediate representation of a terminal recording.
// This is the main output of processing and input for all renderers.
type Recording struct {
	// Metadata from the original cast
	Width    int
	Height   int
	Duration time.Duration
	Title    string

	// Processed data
	Frames []Frame
	Colors *color.ColorCatalog

	// Statistics for renderer optimization
	Stats Stats
}

// Stats holds aggregate information about the recording.
// Renderers can use this to skip generating unused CSS classes.
type Stats struct {
	TotalFrames   int
	UniqueColors  int
	MaxRunsPerRow int // Helps renderers pre-allocate
	HasBold       bool
	HasItalic     bool
	HasUnderline  bool
	HasDim        bool
	HasTrueColor  bool
}

// Cursor represents the cursor state at a point in time.
type Cursor struct {
	Col     int
	Row     int
	Visible bool
}

// Frame represents terminal state at a specific point in time.
type Frame struct {
	// Time is the absolute timestamp from recording start
	Time time.Duration

	// Delay is the time since the previous frame (useful for animation)
	Delay time.Duration

	// Index is the frame number (0-indexed)
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

	// Attrs holds the visual attributes for this run
	Attrs CellAttrs
}

// CellAttrs holds the visual attributes for a cell or run.
type CellAttrs struct {
	// FG and BG are color catalog IDs (not raw colors).
	// Using IDs enables efficient CSS class generation and color deduplication.
	FG color.ColorID
	BG color.ColorID

	// Text styling flags
	Bold      bool
	Italic    bool
	Underline bool
	Dim       bool
}
