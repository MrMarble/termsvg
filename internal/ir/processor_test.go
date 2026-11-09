package ir

import (
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
