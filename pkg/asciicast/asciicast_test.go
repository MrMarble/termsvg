package asciicast_test

import (
	"testing"

	"github.com/mrmarble/termsvg/internal/testutils"
	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/sebdah/goldie/v2"
)

func TestReadRecords(t *testing.T) {
	golden := testutils.GoldenData(t, "TestUnmarshal")

	record, err := asciicast.Unmarshal(golden)
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
			testutils.Diff(t, tc.output, tc.input)
		})
	}
}

func TestWriteRecords(t *testing.T) {
	record := setup(t)

	got, err := record.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	want := goldie.New(t)
	want.AssertWithTemplate(t, "TestMarshal", record, got)
}

func TestToRelativeTime(t *testing.T) {
	cast := setup(t)

	cast.ToRelativeTime()

	for _, event := range cast.Events {
		t.Run(event.EventData, func(t *testing.T) {
			testutils.Diff(t, event.Time, float64(1))
		})
	}
}

func TestCompress(t *testing.T) {
	cast := setup(t)
	cast.Events[1].Time = 1

	cast.Compress()

	testutils.Diff(t, len(cast.Events), 2)
	testutils.Diff(t, cast.Events[0].EventData, "FirstSecond")
	testutils.Diff(t, cast.Events[1].EventData, "Third")
}

func TestToAbsoluteTime(t *testing.T) {
	cast := setup(t)

	cast.ToAbsoluteTime()

	testutils.Diff(t, cast.Events[0].Time, float64(1))
	testutils.Diff(t, cast.Events[1].Time, float64(3))
	testutils.Diff(t, cast.Events[2].Time, float64(6))
}

func TestCapRelativeTime(t *testing.T) {
	cast := setup(t)

	cast.CapRelativeTime(0.5)

	for _, event := range cast.Events {
		t.Run(event.EventData, func(t *testing.T) {
			testutils.Diff(t, event.Time, 0.5)
		})
	}
}

func TestAdjustSpeed(t *testing.T) {
	cast := setup(t)

	cast.AdjustSpeed(2.0)

	testutils.Diff(t, cast.Events[0].Time, float64(0.5))
	testutils.Diff(t, cast.Events[1].Time, float64(1))
	testutils.Diff(t, cast.Events[2].Time, float64(1.5))
}

func setup(t *testing.T) *asciicast.Cast {
	t.Helper()

	t.Setenv("TERM", "TEST_TERM")
	t.Setenv("SHELL", "TEST_SHELL")

	cast := asciicast.New()
	cast.Header.Timestamp = 1337

	cast.Events = append(cast.Events,
		asciicast.Event{Time: 1, EventType: asciicast.Output, EventData: "First"},
		asciicast.Event{Time: 2, EventType: asciicast.Output, EventData: "Second"},
		asciicast.Event{Time: 3, EventType: asciicast.Input, EventData: "Third"},
	)

	return cast
}
