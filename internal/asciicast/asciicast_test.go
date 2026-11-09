package asciicast

import (
	"math"
	"strings"
	"testing"
	"time"
)

const sampleCast = `{"version": 2, "width": 80, "height": 24, "idle_time_limit": 1.5}
[1.0, "o", "a"]
[1.0, "o", "b"]
[3.5, "o", "c"]
`

const v3Palette = "#000000:#111111:#222222:#333333:#444444:#555555:#666666:#777777:" +
	"#888888:#999999:#aaaaaa:#bbbbbb:#cccccc:#dddddd:#eeeeee:#ffffff"

// sampleCastV3 follows the shape of the specification's example: a header with a
// "term" object, relative intervals, a comment line and the o/m/r/x codes.
const sampleCastV3 = `{"version":3,"term":{"cols":80,"rows":24,"type":"xterm-256color","version":"VTE(7802)",` +
	`"theme":{"fg":"#ffffff","bg":"#000000","palette":"` + v3Palette + `"}},` +
	`"timestamp":1504467315,"idle_time_limit":2,"command":"/bin/zsh","title":"Demo",` +
	`"env":{"SHELL":"/bin/zsh"},"tags":["demo"]}
[0.248,"o","Hello "]
[1.001,"o","World!\r\n"]
[3.5,"m",""]
# terminal window resized to 90 cols and 30 rows
[0.143,"r","90x30"]
[1.541,"o","Bye!"]
[0.887,"x","0"]
`

func mustParse(t *testing.T, in string) *Cast {
	t.Helper()
	c, err := Unmarshal([]byte(in))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return c
}

func times(c *Cast) []float64 {
	out := make([]float64, len(c.Events))
	for i, e := range c.Events {
		out[i] = e.Time
	}
	return out
}

func equalFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNew(t *testing.T) {
	t.Setenv("TERM", "xterm-test")
	t.Setenv("SHELL", "/bin/testsh")

	c := New()
	if c.Header.Version != 2 {
		t.Errorf("Version = %d, want 2", c.Header.Version)
	}
	if c.Header.Timestamp == 0 {
		t.Error("Timestamp not set")
	}
	if c.Header.Env["TERM"] != "xterm-test" || c.Header.Env["SHELL"] != "/bin/testsh" {
		t.Errorf("Env = %v", c.Header.Env)
	}
	if c.Events == nil || len(c.Events) != 0 {
		t.Errorf("Events = %v, want empty non-nil", c.Events)
	}
}

func TestMarshal_Roundtrip(t *testing.T) {
	c := mustParse(t, sampleCast)
	data, err := c.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(data), "\n"); lines != len(c.Events) {
		t.Errorf("newline count = %d, want %d", lines, len(c.Events))
	}
	back := mustParse(t, string(data))
	if back.Header.Width != 80 || back.Header.Height != 24 || back.Header.IdleTimeLimit != 1.5 {
		t.Errorf("header = %+v", back.Header)
	}
	if !equalFloats(times(back), times(c)) || len(back.Events) != 3 {
		t.Errorf("events = %v", back.Events)
	}
}

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		wantErr      bool
		wantEvents   int
		wantDuration float64
	}{
		{name: "valid", in: sampleCast, wantEvents: 3, wantDuration: 3.5},
		{
			name:       "explicit duration kept",
			in:         `{"version":2,"width":1,"height":1,"duration":9}` + "\n[1,\"o\",\"x\"]",
			wantEvents: 1, wantDuration: 9,
		},
		{name: "empty input", in: "", wantErr: true},
		{name: "header only", in: `{"version":2,"width":1,"height":1}`, wantErr: true},
		{name: "invalid header json", in: "{nope\n[1,\"o\",\"x\"]", wantErr: true},
		{name: "invalid event json", in: "{\"version\":2}\nnot json", wantErr: true},
		{name: "event wrong arity", in: "{\"version\":2}\n[1,\"o\"]", wantErr: true},
		{name: "event wrong types", in: "{\"version\":2}\n[\"1\",\"o\",\"x\"]", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Unmarshal([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(c.Events) != tt.wantEvents {
				t.Errorf("events = %d, want %d", len(c.Events), tt.wantEvents)
			}
			if c.Header.Duration != tt.wantDuration {
				t.Errorf("duration = %v, want %v", c.Header.Duration, tt.wantDuration)
			}
		})
	}
}

func TestParse(t *testing.T) {
	c, err := Parse(strings.NewReader(sampleCast))
	if err != nil || len(c.Events) != 3 {
		t.Fatalf("Parse() = %v, %v", c, err)
	}
	if _, err := Parse(strings.NewReader("")); err == nil {
		t.Error("Parse(empty) expected error")
	}
}

func TestCast_ToRelativeAndAbsoluteTime(t *testing.T) {
	c := mustParse(t, sampleCast)
	c.ToRelativeTime()
	if want := []float64{1, 0, 2.5}; !equalFloats(times(c), want) {
		t.Errorf("relative = %v, want %v", times(c), want)
	}
	c.ToAbsoluteTime()
	if want := []float64{1, 1, 3.5}; !equalFloats(times(c), want) {
		t.Errorf("absolute = %v, want %v", times(c), want)
	}
}

func TestCast_CapRelativeTime(t *testing.T) {
	tests := []struct {
		name  string
		limit float64
		want  []float64
	}{
		{name: "caps large gaps", limit: 1, want: []float64{1, 0, 1}},
		{name: "zero limit is no-op", limit: 0, want: []float64{1, 0, 2.5}},
		{name: "negative limit is no-op", limit: -1, want: []float64{1, 0, 2.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustParse(t, sampleCast)
			c.ToRelativeTime()
			c.CapRelativeTime(tt.limit)
			if !equalFloats(times(c), tt.want) {
				t.Errorf("times = %v, want %v", times(c), tt.want)
			}
		})
	}
}

func TestCast_AdjustSpeed(t *testing.T) {
	c := mustParse(t, sampleCast)
	c.AdjustSpeed(2)
	if want := []float64{0.5, 0.5, 1.75}; !equalFloats(times(c), want) {
		t.Errorf("times = %v, want %v", times(c), want)
	}
}

func TestCast_Compress(t *testing.T) {
	c := mustParse(t, sampleCast)
	c.Compress()
	if len(c.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(c.Events))
	}
	if c.Events[0].EventData != "ab" || c.Events[1].EventData != "c" {
		t.Errorf("data = %q, %q", c.Events[0].EventData, c.Events[1].EventData)
	}

	empty := &Cast{}
	empty.Compress()
	if len(empty.Events) != 0 {
		t.Errorf("empty compress = %v", empty.Events)
	}
}

func TestUnmarshal_V3(t *testing.T) {
	c := mustParse(t, sampleCastV3)

	h := c.Header
	if h.Width != 80 || h.Height != 24 {
		t.Errorf("size = %dx%d, want 80x24 (from term.cols/rows)", h.Width, h.Height)
	}
	if h.Version != 2 {
		t.Errorf("Version = %d; a parsed Cast follows the v2 model", h.Version)
	}
	if h.Timestamp != 1504467315 || h.IdleTimeLimit != 2 || h.Command != "/bin/zsh" || h.Title != "Demo" ||
		h.Env["SHELL"] != "/bin/zsh" {
		t.Errorf("header = %+v", h)
	}
	if h.Theme.Fg != "#ffffff" || h.Theme.Bg != "#000000" || h.Theme.Palette != v3Palette {
		t.Errorf("theme = %+v, want the one under term.theme", h.Theme)
	}

	wantTypes := []EventType{Output, Output, Marker, Resize, Output, Exit}
	if len(c.Events) != len(wantTypes) {
		t.Fatalf("events = %d, want %d (comment lines are skipped)", len(c.Events), len(wantTypes))
	}
	for i, want := range wantTypes {
		if c.Events[i].EventType != want {
			t.Errorf("event %d type = %q, want %q", i, c.Events[i].EventType, want)
		}
	}
	if c.Events[3].EventData != "90x30" || c.Events[5].EventData != "0" {
		t.Errorf("resize/exit data = %q/%q", c.Events[3].EventData, c.Events[5].EventData)
	}

	// Intervals 0.248, 1.001, 3.5, 0.143, 1.541, 0.887 become absolute offsets.
	want := []float64{0.248, 1.249, 4.749, 4.892, 6.433, 7.32}
	for i, w := range want {
		if got := c.Events[i].Time; math.Abs(got-w) > 1e-9 {
			t.Errorf("event %d time = %v, want %v", i, got, w)
		}
	}
	if math.Abs(h.Duration-7.32) > 1e-9 {
		t.Errorf("Duration = %v, want the last event's time 7.32", h.Duration)
	}
}

func TestUnmarshal_V3MatchesEquivalentV2(t *testing.T) {
	v3 := `{"version":3,"term":{"cols":10,"rows":3}}
[1,"o","a"]
[2,"o","b"]
[0.5,"o","c"]
`
	v2 := `{"version":2,"width":10,"height":3}
[1,"o","a"]
[3,"o","b"]
[3.5,"o","c"]
`
	got, want := mustParse(t, v3), mustParse(t, v2)
	if got.Header.Width != want.Header.Width || got.Header.Height != want.Header.Height ||
		got.Header.Duration != want.Header.Duration {
		t.Errorf("header = %+v, want %+v", got.Header, want.Header)
	}
	if !equalFloats(times(got), times(want)) {
		t.Errorf("times = %v, want %v", times(got), times(want))
	}
}

func TestUnmarshal_V3AcceptsCRLFAndBlankLines(t *testing.T) {
	in := "{\"version\":3,\"term\":{\"cols\":10,\"rows\":3}}\r\n\r\n[1,\"o\",\"a\"]\r\n# note\r\n[1,\"o\",\"b\"]\r\n"
	c := mustParse(t, in)
	if len(c.Events) != 2 || c.Events[1].Time != 2 {
		t.Errorf("events = %+v", c.Events)
	}
}

func TestUnmarshal_V3AndV2RoundTripThroughMarshal(t *testing.T) {
	// A parsed v3 cast is written back out in the v2 model, so it must re-parse
	// to the same events.
	c := mustParse(t, sampleCastV3)
	data, err := c.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back := mustParse(t, string(data))
	if back.Header.Width != 80 || len(back.Events) != len(c.Events) {
		t.Fatalf("re-parsed header=%+v events=%d", back.Header, len(back.Events))
	}
	for i := range c.Events {
		if back.Events[i] != c.Events[i] {
			t.Errorf("event %d = %+v, want %+v", i, back.Events[i], c.Events[i])
		}
	}
}

func TestUnmarshal_HeaderVersions(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{name: "v2", in: `{"version":2,"width":5,"height":2}` + "\n[1,\"o\",\"x\"]"},
		{name: "missing version is treated as v2", in: `{"width":5,"height":2}` + "\n[1,\"o\",\"x\"]"},
		{name: "v3", in: `{"version":3,"term":{"cols":5,"rows":2}}` + "\n[1,\"o\",\"x\"]"},
		{name: "v1", in: `{"version":1,"width":5,"height":2}` + "\n[1,\"o\",\"x\"]", wantErr: "unsupported format version 1"},
		{
			name: "future version", in: `{"version":4,"term":{"cols":5,"rows":2}}` + "\n[1,\"o\",\"x\"]",
			wantErr: "unsupported format version 4",
		},
		{name: "v3 without term", in: `{"version":3}` + "\n[1,\"o\",\"x\"]", wantErr: "term.cols and term.rows"},
		{
			name: "v3 with zero size", in: `{"version":3,"term":{"cols":0,"rows":24}}` + "\n[1,\"o\",\"x\"]",
			wantErr: "term.cols and term.rows",
		},
		{
			name: "v3 with v2 style size", in: `{"version":3,"width":80,"height":24}` + "\n[1,\"o\",\"x\"]",
			wantErr: "term.cols and term.rows",
		},
		{name: "v3 bad header json", in: `{"version":3,"term":}` + "\n[1,\"o\",\"x\"]", wantErr: "invalid character"},
		{name: "v3 bad event", in: `{"version":3,"term":{"cols":5,"rows":2}}` + "\n[1,\"o\"]", wantErr: "3 elements"},
		{name: "v3 without events", in: `{"version":3,"term":{"cols":5,"rows":2}}`, wantErr: "no events"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Unmarshal([]byte(tt.in))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Unmarshal() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestUnmarshal_CommentsOnlySkippedInV3(t *testing.T) {
	v2 := `{"version":2,"width":5,"height":2}` + "\n# not a comment in v2\n[1,\"o\",\"x\"]"
	if _, err := Unmarshal([]byte(v2)); err == nil {
		t.Error("v2 has no comment lines, so '#' should be a parse error")
	}
}

func TestCast_IdleLimit(t *testing.T) {
	withHeader := &Cast{Header: Header{IdleTimeLimit: 1.5}}
	without := &Cast{}
	tests := []struct {
		name     string
		cast     *Cast
		override time.Duration
		want     time.Duration
	}{
		{name: "header applies by default", cast: withHeader, want: 1500 * time.Millisecond},
		{
			name: "explicit override wins over the header", cast: withHeader, override: 200 * time.Millisecond,
			want: 200 * time.Millisecond,
		},
		{name: "a larger override still wins", cast: withHeader, override: 10 * time.Second, want: 10 * time.Second},
		{name: "override without a header", cast: without, override: time.Second, want: time.Second},
		{name: "neither means no limit", cast: without, want: 0},
		{name: "negative override is ignored", cast: withHeader, override: -time.Second, want: 1500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cast.IdleLimit(tt.override); got != tt.want {
				t.Errorf("IdleLimit(%v) = %v, want %v", tt.override, got, tt.want)
			}
		})
	}
}

func TestUnmarshal_V3IdleTimeLimitIsUsable(t *testing.T) {
	c := mustParse(t, sampleCastV3)
	if got := c.IdleLimit(0); got != 2*time.Second {
		t.Errorf("IdleLimit(0) = %v, want 2s from the v3 header", got)
	}
}
