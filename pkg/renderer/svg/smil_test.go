package svg

import (
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestPercentageToUnitIntervalIsExactDecimalShift(t *testing.T) {
	for selector, want := range map[string]string{
		"0%":       "0",
		"0.0001%":  ".000001",
		"1%":       ".01",
		"12.5%":    ".125",
		"50%":      ".5",
		"99.9999%": ".999999",
		"100%":     "1",
	} {
		if got := percentageToUnitInterval(selector); got != want {
			t.Errorf("percentageToUnitInterval(%q) = %q, want %q", selector, got, want)
		}
	}
}

func TestSMILAnimationDurationUsesValidClockSyntax(t *testing.T) {
	for duration, want := range map[time.Duration]string{
		500 * time.Millisecond:                 "0.5s",
		time.Second + time.Nanosecond:          "1.000000001s",
		500*time.Microsecond + time.Nanosecond: "0.500001ms",
	} {
		if got := smilAnimationDuration(duration); got != want {
			t.Errorf("smilAnimationDuration(%v) = %q, want %q", duration, got, want)
		}
	}
}

func TestSMILKeyTimesRetainSelectorsAndEndpoints(t *testing.T) {
	frames := []keyframePoint[int]{
		{selector: "0%"},
		{selector: "0.0001%", state: 1},
		{selector: "50%", state: 2},
		{selector: "100%", state: 2},
	}
	if got, want := smilKeyTimes(frames), "0;.000001;.5;1"; got != want {
		t.Fatalf("smilKeyTimes() = %q, want %q", got, want)
	}
}

func TestSMILRepeatCountMatchesRendererLoopSemantics(t *testing.T) {
	for loopCount, want := range map[int]string{-1: "1", 0: "indefinite", 1: "1", 3: "3"} {
		c := canvas{config: renderer.Config{LoopCount: loopCount}}
		if got := c.smilRepeatCount(); got != want {
			t.Errorf("LoopCount %d = %q, want %q", loopCount, got, want)
		}
	}
}

func TestSMILTranslateValuesAndKeyTimesHaveMatchingLengths(t *testing.T) {
	var out strings.Builder
	c := canvas{
		w:       &out,
		rec:     experimentalRecording(),
		plan:    renderPlan{duration: experimentalRecording().Duration},
		config:  renderer.Config{},
		options: Options{Animation: AnimationSMIL},
	}
	frames := []keyframePoint[int]{
		{selector: "0%", state: 0},
		{selector: "50%", state: 1},
		{selector: "100%", state: 1},
	}
	c.writeSMILTranslate(&out, frames, c.contentWidth())
	got := out.String()
	if !strings.Contains(got, `values="0;-120;-120"`) ||
		!strings.Contains(got, `keyTimes="0;.5;1"`) {
		t.Fatalf("SMIL translate = %s", got)
	}
	values := strings.Split(experimentalAttribute(got, "values"), ";")
	keyTimes := strings.Split(experimentalAttribute(got, "keyTimes"), ";")
	if len(values) != len(keyTimes) {
		t.Fatalf("values=%v keyTimes=%v", values, keyTimes)
	}
}

func experimentalAttribute(markup, name string) string {
	prefix := name + `="`
	start := strings.Index(markup, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.IndexByte(markup[start:], '"')
	if end < 0 {
		return ""
	}
	return markup[start : start+end]
}
