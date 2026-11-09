package ir

import (
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/internal/asciicast"
)

func TestRowsEqual(t *testing.T) {
	run := TextRun{Text: "hi", StartCol: 0, EndCol: 2}
	tests := []struct {
		name string
		a, b Row
		want bool
	}{
		{name: "identical", a: Row{Y: 1, Runs: []TextRun{run}}, b: Row{Y: 1, Runs: []TextRun{run}}, want: true},
		{name: "both empty", a: Row{Y: 2}, b: Row{Y: 2}, want: true},
		{name: "different Y", a: Row{Y: 1, Runs: []TextRun{run}}, b: Row{Y: 2, Runs: []TextRun{run}}},
		{name: "different run count", a: Row{Runs: []TextRun{run}}, b: Row{Runs: []TextRun{run, run}}},
		{name: "different text", a: Row{Runs: []TextRun{run}}, b: Row{Runs: []TextRun{{Text: "ho", EndCol: 2}}}},
		{name: "different attrs", a: Row{Runs: []TextRun{run}}, b: Row{Runs: []TextRun{{
			Text: "hi", EndCol: 2, Attrs: CellAttrs{Bold: true},
		}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rowsEqual(&tt.a, &tt.b); got != tt.want {
				t.Errorf("rowsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFramesEqual_Rows(t *testing.T) {
	row := Row{Y: 0, Runs: []TextRun{{Text: "a", EndCol: 1}}}
	tests := []struct {
		name string
		a, b Frame
		want bool
	}{
		{
			name: "same rows ignoring timing", a: Frame{Rows: []Row{row}, Delay: time.Second},
			b: Frame{Rows: []Row{row}, Delay: time.Hour}, want: true,
		},
		{name: "different row count", a: Frame{Rows: []Row{row}}, b: Frame{Rows: []Row{row, row}}},
		{
			name: "different row content", a: Frame{Rows: []Row{row}},
			b: Frame{Rows: []Row{{Y: 0, Runs: []TextRun{{Text: "b", EndCol: 1}}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := framesEqual(&tt.a, &tt.b); got != tt.want {
				t.Errorf("framesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeduplicateFrames(t *testing.T) {
	a := []Row{{Y: 0, Runs: []TextRun{{Text: "a", EndCol: 1}}}}
	b := []Row{{Y: 0, Runs: []TextRun{{Text: "b", EndCol: 1}}}}

	t.Run("short inputs are returned unchanged", func(t *testing.T) {
		if got := deduplicateFrames(nil); len(got) != 0 {
			t.Errorf("nil -> %v", got)
		}
		one := []Frame{{Rows: a}}
		if got := deduplicateFrames(one); len(got) != 1 {
			t.Errorf("single frame -> %v", got)
		}
	})

	t.Run("merges consecutive duplicates and renumbers", func(t *testing.T) {
		frames := []Frame{
			{Index: 0, Time: 0, Delay: 100 * time.Millisecond, Rows: a},
			{Index: 1, Time: 100 * time.Millisecond, Delay: 200 * time.Millisecond, Rows: a},
			{Index: 2, Time: 300 * time.Millisecond, Delay: 50 * time.Millisecond, Rows: b},
			{Index: 3, Time: 350 * time.Millisecond, Delay: 10 * time.Millisecond, Rows: a},
		}
		got := deduplicateFrames(frames)
		if len(got) != 3 {
			t.Fatalf("frames = %d, want 3", len(got))
		}
		if got[0].Delay != 300*time.Millisecond || got[0].Time != 100*time.Millisecond {
			t.Errorf("merged frame delay=%v time=%v, want 300ms/100ms", got[0].Delay, got[0].Time)
		}
		for i, f := range got {
			if f.Index != i {
				t.Errorf("frame %d has Index %d", i, f.Index)
			}
		}
	})
}

func TestProcessor_StylesAndStats(t *testing.T) {
	const esc = "\x1b["
	tests := []struct {
		name  string
		input string
		want  Stats // only the fields set by styling; frame counts are checked separately
	}{
		{name: "bold", input: esc + "1mA", want: Stats{HasBold: true}},
		{name: "italic", input: esc + "3mA", want: Stats{HasItalic: true}},
		{name: "underline", input: esc + "4mA", want: Stats{HasUnderline: true}},
		{name: "truecolor foreground", input: esc + "38;2;10;20;30mA", want: Stats{HasTrueColor: true, UniqueColors: 1}},
		{name: "truecolor background", input: esc + "48;2;10;20;30mA", want: Stats{HasTrueColor: true, UniqueColors: 1}},
		{name: "ansi colors are not truecolor", input: esc + "31;42mA", want: Stats{UniqueColors: 2}},
		{name: "plain text sets no flags", input: "A", want: Stats{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cast := &asciicast.Cast{
				Header: asciicast.Header{Version: 2, Width: 10, Height: 2},
				Events: []asciicast.Event{{Time: 0, EventType: asciicast.Output, EventData: tt.input}},
			}
			rec, err := NewProcessor(DefaultProcessorConfig()).Process(cast)
			if err != nil {
				t.Fatal(err)
			}
			got := rec.Stats
			got.TotalFrames, got.MaxRunsPerRow = 0, 0
			if got != tt.want {
				t.Errorf("stats = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func lastFrameText(rec *Recording) string {
	var sb strings.Builder
	for _, row := range rec.Frames[len(rec.Frames)-1].Rows {
		for _, run := range row.Runs {
			sb.WriteString(run.Text)
		}
	}
	return sb.String()
}

func TestProcessor_OnlyOutputEventsAreDrawn(t *testing.T) {
	cast := &asciicast.Cast{
		Header: asciicast.Header{Version: 2, Width: 20, Height: 3},
		Events: []asciicast.Event{
			{Time: 0.1, EventType: asciicast.Output, EventData: "ab"},
			{Time: 0.2, EventType: asciicast.Input, EventData: "TYPED"},
			{Time: 0.3, EventType: asciicast.Marker, EventData: "MARK"},
			{Time: 0.4, EventType: asciicast.Resize, EventData: "20x3"}, // same size: no effect
			{Time: 0.5, EventType: asciicast.Output, EventData: "cd"},
			{Time: 0.6, EventType: asciicast.Exit, EventData: "0"},
			{Time: 0.7, EventType: asciicast.EventType("zz"), EventData: "UNKNOWN"},
		},
	}

	rec, err := NewProcessor(DefaultProcessorConfig()).Process(cast)
	if err != nil {
		t.Fatal(err)
	}
	if got := lastFrameText(rec); got != "abcd" {
		t.Errorf("rendered text = %q, want %q (input, marker, resize, exit and unknown events are not output)",
			got, "abcd")
	}
	if len(rec.Frames) != 2 {
		t.Errorf("frames = %d, want 2 (one per output event)", len(rec.Frames))
	}
}

func TestProcessor_RejectsUnusableCasts(t *testing.T) {
	out := []asciicast.Event{{Time: 1, EventType: asciicast.Output, EventData: "x"}}
	tests := []struct {
		name    string
		cast    asciicast.Cast
		wantErr string
	}{
		{
			name: "zero width", cast: asciicast.Cast{Header: asciicast.Header{Width: 0, Height: 5}, Events: out},
			wantErr: "invalid terminal size 0x5",
		},
		{
			name: "zero height", cast: asciicast.Cast{Header: asciicast.Header{Width: 5, Height: 0}, Events: out},
			wantErr: "invalid terminal size 5x0",
		},
		{
			name: "negative size", cast: asciicast.Cast{Header: asciicast.Header{Width: -1, Height: -1}, Events: out},
			wantErr: "invalid terminal size",
		},
		{
			name: "no events", cast: asciicast.Cast{Header: asciicast.Header{Width: 5, Height: 5}},
			wantErr: "no output events",
		},
		{name: "only non-output events", cast: asciicast.Cast{
			Header: asciicast.Header{Width: 5, Height: 5},
			Events: []asciicast.Event{{Time: 1, EventType: asciicast.Exit, EventData: "0"}},
		}, wantErr: "no output events"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := NewProcessor(DefaultProcessorConfig()).Process(&tt.cast)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Process() = %v, %v; want error containing %q", rec, err, tt.wantErr)
			}
		})
	}
}

func outputEvent(at float64, data string) asciicast.Event {
	return asciicast.Event{Time: at, EventType: asciicast.Output, EventData: data}
}

func resizeEvent(at float64, size string) asciicast.Event {
	return asciicast.Event{Time: at, EventType: asciicast.Resize, EventData: size}
}

func processEvents(t *testing.T, cols, rows int, events ...asciicast.Event) *Recording {
	t.Helper()
	cast := &asciicast.Cast{Header: asciicast.Header{Version: 2, Width: cols, Height: rows}, Events: events}
	rec, err := NewProcessor(DefaultProcessorConfig()).Process(cast)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func rowTexts(frame Frame) []string {
	out := make([]string, 0, len(frame.Rows))
	for _, row := range frame.Rows {
		var sb strings.Builder
		for _, run := range row.Runs {
			sb.WriteString(run.Text)
		}
		out = append(out, sb.String())
	}
	return out
}

func TestProcessor_ResizeChangesWrapping(t *testing.T) {
	const text = "0123456789abcdef"
	narrow := processEvents(t, 10, 4, outputEvent(0.1, text))
	wide := processEvents(t, 10, 4, resizeEvent(0.05, "20x4"), outputEvent(0.1, text))

	gotNarrow := rowTexts(narrow.Frames[len(narrow.Frames)-1])
	if len(gotNarrow) != 2 || gotNarrow[0] != "0123456789" || gotNarrow[1] != "abcdef" {
		t.Errorf("10-column terminal rows = %q, want the text wrapped after 10 columns", gotNarrow)
	}
	gotWide := rowTexts(wide.Frames[len(wide.Frames)-1])
	if len(gotWide) != 1 || gotWide[0] != text {
		t.Errorf("terminal resized to 20 columns rows = %q, want the text on one row", gotWide)
	}
}

func TestProcessor_RecordingSizeIsTheLargestSeen(t *testing.T) {
	rec := processEvents(t, 10, 3,
		outputEvent(0.1, "a"),
		resizeEvent(0.2, "30x8"),
		outputEvent(0.3, "b"),
		resizeEvent(0.4, "8x2"),
		outputEvent(0.5, "c"),
	)
	if rec.Width != 30 || rec.Height != 8 {
		t.Errorf("recording size = %dx%d, want 30x8 (the largest terminal size)", rec.Width, rec.Height)
	}
	if got := lastFrameText(rec); got != "abc" {
		t.Errorf("last frame text = %q", got)
	}
}

func TestProcessor_IgnoresInvalidResizes(t *testing.T) {
	invalid := []string{"", "x", "10", "10x", "x10", "axb", "0x5", "5x0", "-3x4", "99999x5", "5x99999", "10 x 5"}
	for _, size := range invalid {
		t.Run(size, func(t *testing.T) {
			rec := processEvents(t, 12, 4, outputEvent(0.1, "a"), resizeEvent(0.2, size), outputEvent(0.3, "b"))
			if rec.Width != 12 || rec.Height != 4 {
				t.Errorf("size = %dx%d after resize %q, want it unchanged at 12x4", rec.Width, rec.Height, size)
			}
			if got := lastFrameText(rec); got != "ab" {
				t.Errorf("text = %q", got)
			}
		})
	}
}

func TestProcessor_ResizeToCurrentSizeAddsNoFrame(t *testing.T) {
	plain := processEvents(t, 12, 4, outputEvent(0.1, "a"), outputEvent(1.0, "b"))
	noop := processEvents(t, 12, 4, outputEvent(0.1, "a"), resizeEvent(0.5, "12x4"), outputEvent(1.0, "b"))
	if len(noop.Frames) != len(plain.Frames) {
		t.Fatalf("frames = %d, want %d", len(noop.Frames), len(plain.Frames))
	}
	for i := range plain.Frames {
		if plain.Frames[i].Time != noop.Frames[i].Time || plain.Frames[i].Delay != noop.Frames[i].Delay {
			t.Errorf("frame %d timing changed by a no-op resize: %v/%v vs %v/%v", i,
				noop.Frames[i].Time, noop.Frames[i].Delay, plain.Frames[i].Time, plain.Frames[i].Delay)
		}
	}
}

func TestProcessor_ResizeIsNotMergedIntoSimultaneousOutput(t *testing.T) {
	// Events at the same instant are compressed into one write, but a resize's
	// "COLSxROWS" data must never be written to the terminal as text.
	rec := processEvents(t, 10, 3, outputEvent(0.1, "a"), resizeEvent(0.1, "20x3"), outputEvent(0.1, "b"))
	if got := lastFrameText(rec); got != "ab" {
		t.Errorf("text = %q, want %q", got, "ab")
	}
	if rec.Width != 20 {
		t.Errorf("Width = %d, want 20", rec.Width)
	}
}

func TestProcessor_ResizeAloneIsNotARecording(t *testing.T) {
	cast := &asciicast.Cast{
		Header: asciicast.Header{Version: 2, Width: 10, Height: 3},
		Events: []asciicast.Event{resizeEvent(0.1, "20x5")},
	}
	if _, err := NewProcessor(DefaultProcessorConfig()).Process(cast); err == nil ||
		!strings.Contains(err.Error(), "no output events") {
		t.Errorf("error = %v, want 'no output events'", err)
	}
}

func TestProcessor_RejectsOversizedHeader(t *testing.T) {
	cast := &asciicast.Cast{
		Header: asciicast.Header{Version: 2, Width: 100000, Height: 24},
		Events: []asciicast.Event{outputEvent(0.1, "a")},
	}
	if _, err := NewProcessor(DefaultProcessorConfig()).Process(cast); err == nil ||
		!strings.Contains(err.Error(), "invalid terminal size") {
		t.Errorf("error = %v, want an invalid terminal size error", err)
	}
}
