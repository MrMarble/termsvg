package asciicast_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mrmarble/termsvg/pkg/asciicast"
)

func TestReadRecords(t *testing.T) {
	record, err := asciicast.ReadRecords("./test_data/test")
	if err != nil {
		t.Fatalf("Error reading: %v", err)
	}

	tests := map[string]struct {
		input  interface{}
		output interface{}
	}{
		"Version":    {input: record.Header.Version, output: 2},
		"Width":      {input: record.Header.Width, output: 213},
		"Height":     {input: record.Header.Height, output: 58},
		"Timestamp":  {input: record.Header.Timestamp, output: int64(1598646467)},
		"Term":       {input: record.Header.Env.Term, output: "alacritty"},
		"Shell":      {input: record.Header.Env.Shell, output: "/usr/bin/zsh"},
		"Event Time": {input: record.Events[0].Time, output: 2.677085},
		"Event Type": {input: record.Events[0].EventType, output: asciicast.Output},
		"Event Data": {input: record.Events[0].EventData, output: "h"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			diff(t, tc.output, tc.input)
		})
	}
}

func TestWriteRecords(t *testing.T) {
	record := setup(t)
	record.Header.Timestamp = 1337

	got, err := record.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		`{"version":2,"width":0,"height":0,"timestamp":1337,"env":{"SHELL":"TEST_SHELL","TERM":"TEST_TERM"}}`,
		`[1,"o","First"]`,
		`[2,"o","Second"]`,
		`[3,"i","Third"]`,
	}

	lines := strings.Split(string(got), "\n")
	for i, line := range lines {
		t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			diff(t, line, want[i])
		})
	}
}

func TestToRelativeTime(t *testing.T) {
	cast := setup(t)

	cast.ToRelativeTime()

	for _, event := range cast.Events {
		t.Run(event.EventData, func(t *testing.T) {
			diff(t, event.Time, float64(1))
		})
	}
}

func TestCompress(t *testing.T) {
	cast := setup(t)
	cast.Events[1].Time = 1

	cast.Compress()

	diff(t, len(cast.Events), 2)
	diff(t, cast.Events[0].EventData, "FirstSecond")
	diff(t, cast.Events[1].EventData, "Third")
}

func TestToAbsoluteTime(t *testing.T) {
	cast := setup(t)

	cast.ToAbsoluteTime()

	diff(t, cast.Events[0].Time, float64(1))
	diff(t, cast.Events[1].Time, float64(3))
	diff(t, cast.Events[2].Time, float64(6))
}

func TestCapRelativeTime(t *testing.T) {
	cast := setup(t)

	cast.CapRelativeTime(0.5)

	for _, event := range cast.Events {
		t.Run(event.EventData, func(t *testing.T) {
			diff(t, event.Time, 0.5)
		})
	}
}

func TestAdjustSpeed(t *testing.T) {
	cast := setup(t)

	cast.AdjustSpeed(2.0)

	diff(t, cast.Events[0].Time, float64(0.5))
	diff(t, cast.Events[1].Time, float64(1))
	diff(t, cast.Events[2].Time, float64(1.5))
}

func setup(t *testing.T) *asciicast.Cast {
	t.Helper()

	t.Setenv("TERM", "TEST_TERM")
	t.Setenv("SHELL", "TEST_SHELL")

	cast := asciicast.NewRecord()

	cast.Events = append(cast.Events,
		asciicast.Event{Time: 1, EventType: asciicast.Output, EventData: "First"},
		asciicast.Event{Time: 2, EventType: asciicast.Output, EventData: "Second"},
		asciicast.Event{Time: 3, EventType: asciicast.Input, EventData: "Third"},
	)

	return cast
}

func diff(t *testing.T, x interface{}, y interface{}) {
	t.Helper()

	diff := cmp.Diff(x, y)
	if diff != "" {
		t.Fatalf(diff)
	}
}
