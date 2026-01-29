package ir

import (
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
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
