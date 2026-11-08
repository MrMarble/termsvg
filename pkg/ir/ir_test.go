package ir

import (
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	termcolor "github.com/mrmarble/termsvg/pkg/color"
)

func TestProcessor_Process(t *testing.T) {
	cast := &asciicast.Cast{
		Header: asciicast.Header{
			Version: 2,
			Width:   80,
			Height:  24,
			Title:   "Test Recording",
		},
		Events: []asciicast.Event{
			{Time: 0.0, EventType: asciicast.Output, EventData: "Hello"},
			{Time: 0.5, EventType: asciicast.Output, EventData: " World"},
			{Time: 1.0, EventType: asciicast.Output, EventData: "!"},
		},
	}

	processor := NewProcessor(DefaultProcessorConfig())
	recording, err := processor.Process(cast)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Check metadata
	if recording.Width != 80 {
		t.Errorf("Width should be 80, got %d", recording.Width)
	}
	if recording.Height != 24 {
		t.Errorf("Height should be 24, got %d", recording.Height)
	}
	if recording.Title != "Test Recording" {
		t.Errorf("Title should be 'Test Recording', got %q", recording.Title)
	}

	// Check frames
	if len(recording.Frames) != 3 {
		t.Errorf("Should have 3 frames, got %d", len(recording.Frames))
	}

	// Check frame timing
	if recording.Frames[0].Time != 0 {
		t.Errorf("First frame time should be 0, got %v", recording.Frames[0].Time)
	}
	if recording.Frames[1].Time != 500*time.Millisecond {
		t.Errorf("Second frame time should be 500ms, got %v", recording.Frames[1].Time)
	}
	if recording.Frames[2].Time != 1*time.Second {
		t.Errorf("Third frame time should be 1s, got %v", recording.Frames[2].Time)
	}

	// Check stats
	if recording.Stats.TotalFrames != 3 {
		t.Errorf("Stats.TotalFrames should be 3, got %d", recording.Stats.TotalFrames)
	}
}

func TestProcessor_Compression(t *testing.T) {
	cast := &asciicast.Cast{
		Header: asciicast.Header{
			Version: 2,
			Width:   80,
			Height:  24,
		},
		Events: []asciicast.Event{
			{Time: 0.0, EventType: asciicast.Output, EventData: "A"},
			{Time: 0.0, EventType: asciicast.Output, EventData: "B"},
			{Time: 0.0, EventType: asciicast.Output, EventData: "C"},
			{Time: 1.0, EventType: asciicast.Output, EventData: "D"},
		},
	}

	config := DefaultProcessorConfig()
	config.Compress = true
	processor := NewProcessor(config)

	recording, err := processor.Process(cast)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Should compress to 2 frames (ABC at 0.0, D at 1.0)
	if len(recording.Frames) != 2 {
		t.Errorf("Should have 2 compressed frames, got %d", len(recording.Frames))
	}
}

func TestProcessor_SpeedAdjustment(t *testing.T) {
	cast := &asciicast.Cast{
		Header: asciicast.Header{
			Version: 2,
			Width:   80,
			Height:  24,
		},
		Events: []asciicast.Event{
			{Time: 0.0, EventType: asciicast.Output, EventData: "A"},
			{Time: 2.0, EventType: asciicast.Output, EventData: "B"},
		},
	}

	config := DefaultProcessorConfig()
	config.Speed = 2.0 // 2x speed
	processor := NewProcessor(config)

	recording, err := processor.Process(cast)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// At 2x speed, 2.0s becomes 1.0s
	if recording.Frames[1].Time != 1*time.Second {
		t.Errorf("At 2x speed, 2s should become 1s, got %v", recording.Frames[1].Time)
	}
}

func TestProcessor_IdleTimeCap(t *testing.T) {
	cast := &asciicast.Cast{
		Header: asciicast.Header{
			Version: 2,
			Width:   80,
			Height:  24,
		},
		Events: []asciicast.Event{
			{Time: 0.0, EventType: asciicast.Output, EventData: "A"},
			{Time: 10.0, EventType: asciicast.Output, EventData: "B"}, // 10s gap
			{Time: 11.0, EventType: asciicast.Output, EventData: "C"}, // 1s gap
		},
	}

	config := DefaultProcessorConfig()
	config.IdleTimeLimit = 2 * time.Second // Cap to 2s
	processor := NewProcessor(config)

	recording, err := processor.Process(cast)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// First gap should be capped from 10s to 2s
	// Second frame should be at 2s (not 10s)
	if recording.Frames[1].Time != 2*time.Second {
		t.Errorf("Second frame should be at 2s after capping, got %v", recording.Frames[1].Time)
	}

	// Third frame should be at 3s (2s + 1s)
	if recording.Frames[2].Time != 3*time.Second {
		t.Errorf("Third frame should be at 3s, got %v", recording.Frames[2].Time)
	}
}

//nolint:funlen // table cases keep idle-time edge conditions together.
func TestProcessor_PreprocessEventsIdleTimeCap(t *testing.T) {
	tests := []struct {
		name   string
		config func(*ProcessorConfig)
		events []asciicast.Event
		want   []float64
	}{
		{
			name: "multiple long gaps",
			config: func(config *ProcessorConfig) {
				config.IdleTimeLimit = 2 * time.Second
			},
			events: []asciicast.Event{
				{Time: 0}, {Time: 10}, {Time: 20}, {Time: 21},
			},
			want: []float64{0, 2, 4, 5},
		},
		{
			name: "gap equal to limit",
			config: func(config *ProcessorConfig) {
				config.IdleTimeLimit = 2 * time.Second
			},
			events: []asciicast.Event{
				{Time: 0}, {Time: 2}, {Time: 4},
			},
			want: []float64{0, 2, 4},
		},
		{
			name: "decimal gap below limit",
			config: func(config *ProcessorConfig) {
				config.IdleTimeLimit = 100 * time.Millisecond
			},
			events: []asciicast.Event{
				{Time: 0}, {Time: math.Nextafter(0.1, 0)},
			},
			want: []float64{0, math.Nextafter(0.1, 0)},
		},
		{
			name: "decimal gap above limit",
			config: func(config *ProcessorConfig) {
				config.IdleTimeLimit = 100 * time.Millisecond
			},
			events: []asciicast.Event{
				{Time: 0}, {Time: math.Nextafter(0.1, math.Inf(1))},
			},
			want: []float64{0, 0.1},
		},
		{
			name: "speed before capping",
			config: func(config *ProcessorConfig) {
				config.Speed = 2
				config.IdleTimeLimit = 2 * time.Second
			},
			events: []asciicast.Event{
				{Time: 0}, {Time: 10}, {Time: 14},
			},
			want: []float64{0, 2, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultProcessorConfig()
			config.Compress = false
			tt.config(config)
			cast := &asciicast.Cast{Events: tt.events}
			sourceEvents := append([]asciicast.Event(nil), tt.events...)

			got := NewProcessor(config).preprocessEvents(cast)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d events, want %d", len(got), len(tt.want))
			}
			for i, want := range tt.want {
				if got[i].Time != want {
					t.Errorf("event %d time = %v, want %v", i, got[i].Time, want)
				}
				if cast.Events[i] != sourceEvents[i] {
					t.Errorf("source event %d = %#v, want unchanged %#v", i, cast.Events[i], sourceEvents[i])
				}
			}
		})
	}
}

func TestTextRunGrouping(t *testing.T) {
	cast := &asciicast.Cast{
		Header: asciicast.Header{
			Version: 2,
			Width:   10,
			Height:  1,
		},
		Events: []asciicast.Event{
			// Write some text - all same attributes, should be one run
			{Time: 0.0, EventType: asciicast.Output, EventData: "Hello"},
		},
	}

	processor := NewProcessor(DefaultProcessorConfig())
	recording, err := processor.Process(cast)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// First row should have runs that group consecutive same-attribute cells
	row := recording.Frames[0].Rows[0]
	if len(row.Runs) == 0 {
		t.Fatal("Should have at least one run")
	}

	// The "Hello" text should be in the first run (or grouped somehow)
	foundHello := false
	for _, run := range row.Runs {
		if len(run.Text) >= 5 && run.Text[:5] == "Hello" {
			foundHello = true
			break
		}
	}
	if !foundHello {
		t.Errorf("Should find 'Hello' in first run, got runs: %+v", row.Runs)
	}
}

func TestProcessor_TextRunCellExtents(t *testing.T) {
	cast := &asciicast.Cast{
		Header: asciicast.Header{Version: 2, Width: 10, Height: 1},
		Events: []asciicast.Event{
			{Time: 0, EventType: asciicast.Output, EventData: "ab\x1b[31mc"},
		},
	}

	recording, err := NewProcessor(DefaultProcessorConfig()).Process(cast)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	runs := recording.Frames[0].Rows[0].Runs
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2: %#v", len(runs), runs)
	}
	if got, want := runs[0].EndCol, 2; got != want {
		t.Errorf("first EndCol = %d, want %d", got, want)
	}
	if got, want := runs[1].StartCol, runs[0].EndCol; got != want {
		t.Errorf("adjacent boundary = %d, want %d", got, want)
	}
	if got, want := runs[1].EndCol, 3; got != want {
		t.Errorf("second EndCol = %d, want %d", got, want)
	}
}

func TestCleanRuns_TrimTrailingSpacesUpdatesEndCol(t *testing.T) {
	catalog := termcolor.NewCatalog(color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{A: 255})
	runs := []TextRun{{Text: "text  ", StartCol: 3, EndCol: 9}}

	got := cleanRuns(runs, catalog)
	if got[0].Text != "text" || got[0].EndCol != 7 {
		t.Fatalf("cleanRuns() = %#v, want trimmed run ending at column 7", got)
	}
}

func TestTextRunsEqual_IncludesEndCol(t *testing.T) {
	a := TextRun{Text: "same", StartCol: 3, EndCol: 7}
	b := a
	b.EndCol = 8

	if textRunsEqual(&a, &b) {
		t.Fatal("textRunsEqual() ignored EndCol")
	}
}

func TestTextRun_ExplicitWideCellExtent(t *testing.T) {
	run := TextRun{Text: "界", StartCol: 3, EndCol: 5}
	if got, want := run.EndCol-run.StartCol, 2; got != want {
		t.Errorf("cell extent = %d, want %d", got, want)
	}
}

func TestAttrsEqual(t *testing.T) {
	a := CellAttrs{FG: 1, BG: 2, Bold: true}
	b := CellAttrs{FG: 1, BG: 2, Bold: true}
	c := CellAttrs{FG: 1, BG: 2, Bold: false}

	if !attrsEqual(a, b) {
		t.Error("Same attrs should be equal")
	}
	if attrsEqual(a, c) {
		t.Error("Different attrs should not be equal")
	}
}

func TestFramesEqual_CursorChange(t *testing.T) {
	rows := []Row{{Y: 0, Runs: []TextRun{{Text: "same"}}}}
	base := Frame{Rows: rows, Cursor: Cursor{Col: 4, Row: 0, Visible: true}}
	tests := []struct {
		name   string
		cursor Cursor
	}{
		{name: "position", cursor: Cursor{Col: 5, Row: 0, Visible: true}},
		{name: "visibility", cursor: Cursor{Col: 4, Row: 0, Visible: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := Frame{Rows: rows, Cursor: tt.cursor}
			if framesEqual(&base, &changed) {
				t.Fatal("framesEqual() ignored a cursor change")
			}
		})
	}
}

func TestCleanRuns_PreservesUnderlinedWhitespace(t *testing.T) {
	catalog := termcolor.NewCatalog(
		color.RGBA{R: 255, G: 255, B: 255, A: 255},
		color.RGBA{A: 255},
	)
	runs := []TextRun{{Text: "   ", Attrs: CellAttrs{Underline: true}}}

	got := cleanRuns(runs, catalog)
	if len(got) != 1 || got[0].Text != "   " {
		t.Fatalf("cleanRuns() = %#v, want visible underlined spaces", got)
	}
}
