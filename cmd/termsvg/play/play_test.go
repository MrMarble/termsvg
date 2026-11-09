package play

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/internal/asciicast"
)

const testCast = `{"version":2,"width":80,"height":24}
[0.01,"o","a"]
[0.02,"o","b"]
[0.50,"o","c"]
`

// captureStdout runs fn while redirecting os.Stdout and returns what was printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	func() {
		defer func() { os.Stdout = orig }()
		fn()
	}()
	_ = w.Close()
	return <-done
}

func parse(t *testing.T) *asciicast.Cast {
	t.Helper()
	c, err := asciicast.Parse(strings.NewReader(testCast))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// mustPlayback plays the test cast, failing the test on error, and returns stdout.
func mustPlayback(t *testing.T, speed float64, maxIdle time.Duration) string {
	t.Helper()
	var err error
	out := captureStdout(t, func() { err = playback(parse(t), speed, maxIdle) })
	if err != nil {
		t.Fatalf("playback() error = %v", err)
	}
	return out
}

func TestPlayback_PrintsAllEventsInOrder(t *testing.T) {
	var err error
	out := captureStdout(t, func() { err = playback(parse(t), 100, 0) })
	if err != nil {
		t.Fatal(err)
	}
	if out != "abc" {
		t.Errorf("output = %q, want %q", out, "abc")
	}
}

func TestPlayback_SpeedShortensDuration(t *testing.T) {
	start := time.Now()
	mustPlayback(t, 50, 0)
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Errorf("playback at 50x took %v, expected well under the 500ms real duration", d)
	}
}

func TestPlayback_MaxIdleCapsGaps(t *testing.T) {
	start := time.Now()
	out := mustPlayback(t, 1, 20*time.Millisecond)
	d := time.Since(start)
	if out != "abc" {
		t.Errorf("output = %q", out)
	}
	if d > 300*time.Millisecond {
		t.Errorf("idle cap ignored: took %v", d)
	}
}

func TestPlayback_RespectsRealTiming(t *testing.T) {
	start := time.Now()
	mustPlayback(t, 1, 0)
	if d := time.Since(start); d < 450*time.Millisecond {
		t.Errorf("playback finished too early: %v", d)
	}
}

func TestCmd_Run(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.cast")
	bad := filepath.Join(dir, "bad.cast")
	if err := os.WriteFile(good, []byte(strings.ReplaceAll(testCast, "0.50", "0.03")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("not a cast"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		cmd     Cmd
		want    string
		wantErr bool
	}{
		{name: "plays file", cmd: Cmd{File: good, Speed: 10}, want: "abc"},
		{name: "invalid cast", cmd: Cmd{File: bad, Speed: 1}, wantErr: true},
		{name: "missing file", cmd: Cmd{File: filepath.Join(dir, "none.cast"), Speed: 1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			out := captureStdout(t, func() { err = tt.cmd.Run() })
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if out != tt.want {
				t.Errorf("output = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestCmd_Run_UsesRecordingIdleTimeLimit(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "idle.cast")
	content := `{"version":2,"width":20,"height":3,"idle_time_limit":0.02}` + "\n" +
		`[0.01,"o","a"]` + "\n" + `[2.01,"o","b"]` + "\n"
	if err := os.WriteFile(in, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	var err error
	out := captureStdout(t, func() { err = (&Cmd{File: in, Speed: 1}).Run() })
	if err != nil {
		t.Fatal(err)
	}
	if out != "ab" {
		t.Errorf("output = %q", out)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("playback took %v; the 2s pause should have been capped by the header", elapsed)
	}
}

func TestPlayback_OnlyOutputEventsArePrinted(t *testing.T) {
	cast, err := asciicast.Parse(strings.NewReader(`{"version":3,"term":{"cols":80,"rows":24}}
[0.001,"o","a"]
[0.001,"i","TYPED"]
[0.001,"m","MARK"]
[0.001,"r","100x50"]
[0.001,"o","b"]
[0.001,"x","0"]
`))
	if err != nil {
		t.Fatal(err)
	}
	var playErr error
	out := captureStdout(t, func() { playErr = playback(cast, 100, 0) })
	if playErr != nil {
		t.Fatal(playErr)
	}
	if out != "ab" {
		t.Errorf("output = %q, want %q", out, "ab")
	}
}
