package svg

import (
	"reflect"
	"testing"
	"time"
)

func TestQuantizeTimelineLastStateInBucketWins(t *testing.T) {
	timeline := normalizeTimeline(time.Second, []timelinePoint[int]{
		{time: 0, state: 0},
		{time: 100 * time.Millisecond, state: 1},
		{time: 400 * time.Millisecond, state: 2},
		{time: 510 * time.Millisecond, state: 3},
		{time: 900 * time.Millisecond, state: 4},
		{time: time.Second, state: 5},
	}, func(a, b int) bool { return a == b })

	got := quantizeTimeline(timeline, 2, func(a, b int) bool { return a == b })
	want := []timelinePoint[int]{
		{time: 0, state: 0},
		{time: 500 * time.Millisecond, state: 2},
		{time: time.Second, state: 5},
	}
	if !reflect.DeepEqual(got.points, want) {
		t.Fatalf("quantized points = %#v, want %#v", got.points, want)
	}
}

func TestQuantizeTimelineKeepsExactBucketBoundary(t *testing.T) {
	timeline := normalizeTimeline(time.Second, []timelinePoint[int]{
		{time: 0, state: 0},
		{time: 500 * time.Millisecond, state: 1},
		{time: time.Second, state: 2},
	}, func(a, b int) bool { return a == b })
	got := quantizeTimeline(timeline, 2, func(a, b int) bool { return a == b })
	if !reflect.DeepEqual(got.points, timeline.points) {
		t.Fatalf("boundary points = %#v, want %#v", got.points, timeline.points)
	}
}

func TestQuantizeTimelinePreservesInitialAndFinalShortRecordingStates(t *testing.T) {
	timeline := normalizeTimeline(100*time.Millisecond, []timelinePoint[int]{
		{time: 0, state: 0},
		{time: 90 * time.Millisecond, state: 1},
	}, func(a, b int) bool { return a == b })
	got := quantizeTimeline(timeline, 1, func(a, b int) bool { return a == b })
	want := []timelinePoint[int]{{time: 0, state: 0}, {time: 100 * time.Millisecond, state: 1}}
	if !reflect.DeepEqual(got.points, want) {
		t.Fatalf("short recording points = %#v, want %#v", got.points, want)
	}
}

func TestQuantizeTimelineZeroIsLossless(t *testing.T) {
	timeline := normalizeTimeline(time.Second, []timelinePoint[int]{
		{time: 0, state: 0},
		{time: 123 * time.Millisecond, state: 1},
		{time: 456 * time.Millisecond, state: 2},
	}, func(a, b int) bool { return a == b })
	got := quantizeTimeline(timeline, 0, func(a, b int) bool { return a == b })
	if !reflect.DeepEqual(got, timeline) {
		t.Fatalf("zero-FPS timeline = %#v, want %#v", got, timeline)
	}
}
