package asciicast

import (
	"bytes"
	"strings"
	"testing"
)

const sampleCast = `{"version": 2, "width": 80, "height": 24, "idle_time_limit": 1.5}
[1.0, "o", "a"]
[1.0, "o", "b"]
[3.5, "o", "c"]
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

func TestCast_WriteTo(t *testing.T) {
	c := mustParse(t, sampleCast)
	var buf bytes.Buffer
	n, err := c.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(buf.Len()) || buf.Len() == 0 {
		t.Errorf("n = %d, buffer = %d", n, buf.Len())
	}
	if _, err := Parse(&buf); err != nil {
		t.Errorf("written output does not parse: %v", err)
	}
}
