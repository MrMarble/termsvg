package ir

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
)

func TestProcessorIdleCapPreservesLegacyFloatRounding(t *testing.T) {
	events := []asciicast.Event{
		{Time: 0.03, EventData: "a"},
		{Time: 0.05, EventData: "b"},
		{Time: 0.060000000000000005, EventData: "c"},
	}
	cast := &asciicast.Cast{Events: events}
	config := DefaultProcessorConfig()
	config.Compress = false
	config.IdleTimeLimit = 20 * time.Millisecond

	got := NewProcessor(config).preprocessEvents(cast)
	want := legacyPreprocessEvents(config, cast)
	assertEventsBitEqual(t, got, want)

	wantLast := math.Nextafter(0.05, 0)
	if math.Float64bits(got[2].Time) != math.Float64bits(wantLast) {
		t.Fatalf("last timestamp bits = %#x (%0.17g), want %#x (%0.17g)",
			math.Float64bits(got[2].Time), got[2].Time,
			math.Float64bits(wantLast), wantLast)
	}
	if !reflect.DeepEqual(cast.Events, events) {
		t.Fatalf("preprocessEvents mutated source events: got %#v, want %#v", cast.Events, events)
	}
}

func TestProcessorIdleCapPreservesExactEqualityCompression(t *testing.T) {
	cast := &asciicast.Cast{Events: []asciicast.Event{
		{Time: 0.008, EventData: "a"},
		{Time: 0.068, EventData: "b"},
		{Time: 0.078, EventData: "c"},
		{Time: 0.278, EventData: "d"},
		{Time: 0.378, EventData: "e"},
		{Time: 0.478, EventData: "f"},
		{Time: 0.478, EventData: "g"},
	}}
	config := DefaultProcessorConfig()
	config.Compress = true
	config.IdleTimeLimit = 30 * time.Millisecond

	got := NewProcessor(config).preprocessEvents(cast)
	want := legacyPreprocessEvents(config, cast)
	assertEventsBitEqual(t, got, want)
	if len(got) != 6 || got[len(got)-1].EventData != "fg" {
		t.Fatalf("compressed events = %#v, want six events ending in merged data %q", got, "fg")
	}
}

func TestProcessorIdleCapMatchesLegacyReference(t *testing.T) {
	rng := rand.New(rand.NewSource(0x7465726d737667)) //nolint:gosec // deterministic test data.
	for testCase := 0; testCase < 2_000; testCase++ {
		count := 1 + rng.Intn(20)
		events := make([]asciicast.Event, count)
		current := rng.Float64()
		for i := range events {
			gap := rng.Float64() * float64(1+rng.Intn(20))
			if rng.Intn(5) == 0 {
				gap = math.Nextafter(gap, math.Inf(1))
			}
			current += gap
			events[i] = asciicast.Event{Time: current, EventData: string(rune('a' + i%26))}
		}
		cast := &asciicast.Cast{Events: events}
		config := DefaultProcessorConfig()
		config.Speed = []float64{0.5, 1, 1.5, 2, 3}[rng.Intn(5)]
		config.IdleTimeLimit = time.Duration(1+rng.Intn(2_000)) * time.Millisecond
		config.Compress = rng.Intn(2) == 0

		got := NewProcessor(config).preprocessEvents(cast)
		want := legacyPreprocessEvents(config, cast)
		assertEventsBitEqual(t, got, want)
		if !reflect.DeepEqual(cast.Events, events) {
			t.Fatalf("case %d mutated source events", testCase)
		}
	}
}

func legacyPreprocessEvents(config *ProcessorConfig, cast *asciicast.Cast) []asciicast.Event {
	events := append([]asciicast.Event(nil), cast.Events...)
	if config.Speed != 1 && config.Speed > 0 {
		for i := range events {
			events[i].Time /= config.Speed
		}
	}
	if config.IdleTimeLimit > 0 {
		limit := config.IdleTimeLimit.Seconds()
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
	if config.Compress {
		compressed := make([]asciicast.Event, 0, len(events))
		for _, event := range events {
			if len(compressed) > 0 && event.Time == compressed[len(compressed)-1].Time {
				compressed[len(compressed)-1].EventData += event.EventData
				continue
			}
			compressed = append(compressed, event)
		}
		events = compressed
	}
	return events
}

func assertEventsBitEqual(t *testing.T, got, want []asciicast.Event) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event count = %d, want %d\ngot:  %#v\nwant: %#v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].EventType != want[i].EventType || got[i].EventData != want[i].EventData {
			t.Fatalf("event %d = %#v, want %#v", i, got[i], want[i])
		}
		if math.Float64bits(got[i].Time) != math.Float64bits(want[i].Time) {
			t.Fatalf("event %d timestamp bits = %#x (%0.17g), want %#x (%0.17g)",
				i, math.Float64bits(got[i].Time), got[i].Time,
				math.Float64bits(want[i].Time), want[i].Time)
		}
	}
}
