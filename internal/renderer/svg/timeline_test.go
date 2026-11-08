package svg

import (
	"reflect"
	"testing"
	"time"
)

func TestNormalizeTimelineOrdersAndCompactsStates(t *testing.T) {
	got := normalizeTimeline(4*time.Second, []timelinePoint[string]{
		{time: 2 * time.Second, state: "middle"},
		{time: time.Second, state: "old"},
		{time: time.Second, state: "first"},
		{time: 3 * time.Second, state: "middle"},
	}, func(a, b string) bool { return a == b })
	want := timeline[string]{duration: 4 * time.Second, points: []timelinePoint[string]{
		{time: 0, state: "first"},
		{time: 2 * time.Second, state: "middle"},
		{time: 4 * time.Second, state: "middle"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
}

func TestNormalizeTimelineCompactsAfterSameTimeReplacement(t *testing.T) {
	got := normalizeTimeline(time.Second, []timelinePoint[string]{
		{state: "same"},
		{time: 500 * time.Millisecond, state: "discarded"},
		{time: 500 * time.Millisecond, state: "same"},
	}, func(a, b string) bool { return a == b })
	want := timeline[string]{duration: time.Second, points: []timelinePoint[string]{{state: "same"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
}

func TestNormalizeTimelineClampsPointsToEndpoints(t *testing.T) {
	got := normalizeTimeline(2*time.Second, []timelinePoint[string]{
		{time: -time.Second, state: "old start"},
		{state: "start"},
		{time: time.Second, state: "middle"},
		{time: 3 * time.Second, state: "old end"},
		{time: 4 * time.Second, state: "end"},
	}, func(a, b string) bool { return a == b })
	want := timeline[string]{duration: 2 * time.Second, points: []timelinePoint[string]{
		{state: "start"},
		{time: time.Second, state: "middle"},
		{time: 2 * time.Second, state: "end"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
	for _, frame := range got.keyframes() {
		if frame.selector[0] == '-' || frame.selector[0] > '9' {
			t.Fatalf("out-of-range selector %q", frame.selector)
		}
	}
}

func TestNormalizeTimelineClampsAfterOrderingOriginalTimes(t *testing.T) {
	got := normalizeTimeline(2*time.Second, []timelinePoint[string]{
		{time: 4 * time.Second, state: "end"},
		{time: 3 * time.Second, state: "old"},
		{time: -3 * time.Second, state: "start"},
		{time: -4 * time.Second, state: "older"},
	}, func(a, b string) bool { return a == b })
	want := timeline[string]{duration: 2 * time.Second, points: []timelinePoint[string]{
		{state: "start"},
		{time: 2 * time.Second, state: "end"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
}

func TestNormalizeTimelineMakesStaticAndZeroDurationTimelinesStatic(t *testing.T) {
	for _, tt := range []struct {
		name     string
		duration time.Duration
		points   []timelinePoint[int]
		want     timeline[int]
	}{
		{
			name:     "one state",
			duration: time.Second,
			points:   []timelinePoint[int]{{time: 500 * time.Millisecond, state: 1}},
			want:     timeline[int]{duration: time.Second, points: []timelinePoint[int]{{state: 1}}},
		},
		{
			name:   "zero duration uses final state",
			points: []timelinePoint[int]{{state: 1}, {time: time.Second, state: 2}},
			want:   timeline[int]{points: []timelinePoint[int]{{state: 2}}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTimeline(tt.duration, tt.points, func(a, b int) bool { return a == b })
			if !reflect.DeepEqual(got, tt.want) || got.animated() {
				t.Fatalf("timeline = %#v, animated=%v; want %#v, false", got, got.animated(), tt.want)
			}
		})
	}
}

func TestTimelineSelectorsAreUniqueAndDeterministic(t *testing.T) {
	timeline := timeline[string]{duration: time.Duration(1<<63 - 1), points: []timelinePoint[string]{
		{state: "a"},
		{time: time.Nanosecond, state: "b"},
		{time: 2 * time.Nanosecond, state: "c"},
		{time: time.Duration(1<<63 - 1), state: "c"},
	}}
	want := []keyframePoint[string]{
		{selector: "0%", state: "c"},
	}
	if got := timeline.keyframes(func(a, b string) bool { return a == b }); !reflect.DeepEqual(got, want) {
		t.Fatalf("keyframes = %#v, want %#v", got, want)
	}
}

func TestTimelineSelectorsIncreaseOnlyRequiredPrecision(t *testing.T) {
	timeline := timeline[string]{duration: time.Second, points: []timelinePoint[string]{
		{state: "a"},
		{time: time.Microsecond, state: "b"},
		{time: 2 * time.Microsecond, state: "c"},
		{time: time.Second, state: "c"},
	}}
	want := []keyframePoint[string]{
		{selector: "0%", state: "a"},
		{selector: "0.0001%", state: "b"},
		{selector: "0.0002%", state: "c"},
		{selector: "100%", state: "c"},
	}
	first := timeline.keyframes()
	if !reflect.DeepEqual(first, want) || !reflect.DeepEqual(timeline.keyframes(), first) {
		t.Fatalf("keyframes = %#v, want deterministic %#v", first, want)
	}
}

func TestAnimationDurationChoosesShortestExactCSSValue(t *testing.T) {
	for duration, want := range map[time.Duration]string{
		0:                                       "0s",
		-500 * time.Millisecond:                 "-.5s",
		500 * time.Millisecond:                  ".5s",
		1500 * time.Millisecond:                 "1.5s",
		time.Second + time.Nanosecond:           "1.000000001s",
		500*time.Microsecond + time.Nanosecond:  ".500001ms",
		-500*time.Microsecond - time.Nanosecond: "-.500001ms",
	} {
		if got := animationDuration(duration); got != want {
			t.Errorf("animationDuration(%v) = %q, want %q", duration, got, want)
		}
	}
}
