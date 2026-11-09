package ir

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/internal/asciicast"
)

func outputEvents(times ...float64) []asciicast.Event {
	events := make([]asciicast.Event, len(times))
	for i, t := range times {
		events[i] = asciicast.Event{Time: t, EventType: asciicast.Output, EventData: string(rune('a' + i))}
	}

	return events
}

// Capping subtracts each pause's excess from every later event, so the
// rounding of those subtractions decides whether two times end up equal.
func TestPreprocessEvents_IdleCapKeepsFloatRounding(t *testing.T) {
	cast := &asciicast.Cast{Events: outputEvents(0.03, 0.05, 0.060000000000000005)}
	config := DefaultProcessorConfig()
	config.Compress = false
	config.IdleTimeLimit = 20 * time.Millisecond

	got := NewProcessor(config).preprocessEvents(cast)

	wantLast := math.Nextafter(0.05, 0)
	if math.Float64bits(got[2].Time) != math.Float64bits(wantLast) {
		t.Fatalf("last timestamp = %0.17g, want %0.17g", got[2].Time, wantLast)
	}
	if !reflect.DeepEqual(cast.Events, outputEvents(0.03, 0.05, 0.060000000000000005)) {
		t.Fatal("preprocessEvents mutated the source events")
	}
}

func TestPreprocessEvents_CompressMergesEventsAtTheSameInstant(t *testing.T) {
	cast := &asciicast.Cast{Events: outputEvents(0.008, 0.068, 0.078, 0.278, 0.378, 0.478, 0.478)}
	config := DefaultProcessorConfig()
	config.IdleTimeLimit = 30 * time.Millisecond

	got := NewProcessor(config).preprocessEvents(cast)

	if len(got) != 6 || got[len(got)-1].EventData != "fg" {
		t.Fatalf("events = %#v, want six events ending in merged data %q", got, "fg")
	}
}
