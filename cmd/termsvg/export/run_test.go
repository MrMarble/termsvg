package export

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const runTestPalette = "#000000:#111111:#222222:#333333:#444444:#555555:#666666:#777777:" +
	"#888888:#999999:#aaaaaa:#bbbbbb:#cccccc:#dddddd:#eeeeee:#ffffff"

const runTestCast = `{"version":2,"width":20,"height":5}
[0.1,"o","hello"]
[0.2,"o","\r\n\u001b[31mred\u001b[0m"]
[0.4,"o","\r\nworld"]
`

func writeRunFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// newCmd returns a Cmd carrying the defaults Kong would apply.
func newCmd(file, output, format string) *Cmd {
	return &Cmd{
		File: file, Output: output, Format: format, Speed: 1,
		SVGLayout: "frames", SVGAnimation: "css", SVGFrameSwitch: "translate",
		SVGAutoObjective: "size", SVGStyle: "legacy", SVGPrimitives: "snapshots",
	}
}

func TestCmd_Run_SVG(t *testing.T) {
	dir := t.TempDir()
	in := writeRunFile(t, dir, "in.cast", runTestCast)

	tests := []struct {
		name   string
		tweak  func(*Cmd)
		output string
	}{
		{name: "defaults", tweak: func(*Cmd) {}},
		{name: "minified", tweak: func(c *Cmd) { c.Minify = true }},
		{name: "no window and cursor", tweak: func(c *Cmd) { c.NoWindow, c.NoCursor = true, true }},
		{name: "size override and speed", tweak: func(c *Cmd) { c.Cols, c.Rows, c.Speed = 30, 8, 2 }},
		{name: "max idle", tweak: func(c *Cmd) { c.MaxIdle = 50 * time.Millisecond }},
		{name: "debug", tweak: func(c *Cmd) { c.Debug = true }},
		{name: "default output path", tweak: func(c *Cmd) { c.Output = "" }, output: in + ".svg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.svg")
			cmd := newCmd(in, out, "svg")
			tt.tweak(cmd)
			if tt.output != "" {
				out = tt.output
				t.Cleanup(func() { _ = os.Remove(out) })
			}
			if err := cmd.Run(); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			data, err := os.ReadFile(out) //nolint:gosec // test temp file
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "<svg") {
				t.Errorf("output is not an SVG: %.80s", data)
			}
		})
	}
}

func TestCmd_Run_OtherFormats(t *testing.T) {
	in := writeRunFile(t, t.TempDir(), "in.cast", runTestCast)
	tests := []struct {
		format string
		magic  string
	}{
		{format: "gif", magic: "GIF8"},
		{format: "webm", magic: "\x1a\x45\xdf\xa3"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			if tt.format == "webm" {
				if _, err := exec.LookPath("ffmpeg"); err != nil {
					t.Skip("ffmpeg not installed")
				}
			}
			out := filepath.Join(t.TempDir(), "out."+tt.format)
			if err := newCmd(in, out, tt.format).Run(); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			data, err := os.ReadFile(out) //nolint:gosec // test temp file
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(data), tt.magic) {
				t.Errorf("unexpected file header %q", data[:min(8, len(data))])
			}
		})
	}
}

func TestCmd_Run_Themes(t *testing.T) {
	dir := t.TempDir()
	plain := writeRunFile(t, dir, "plain.cast", runTestCast)
	withTheme := writeRunFile(t, dir, "themed.cast",
		`{"version":2,"width":20,"height":5,`+
			`"theme":{"fg":"#112233","bg":"#445566","palette":"`+runTestPalette+`"}}`+"\n[0.1,\"o\",\"x\"]\n")
	badTheme := writeRunFile(t, dir, "badtheme.cast",
		`{"version":2,"width":20,"height":5,"theme":{"fg":"zz","bg":"#445566","palette":"x"}}`+"\n[0.1,\"o\",\"x\"]\n")
	themeFile := writeRunFile(t, dir, "mine.json", `{"fg":"#aabbcc","bg":"#010203","palette":"`+runTestPalette+`"}`)

	tests := []struct {
		name  string
		file  string
		theme string
		want  string // substring expected in the SVG (lowercase hex)
	}{
		{name: "builtin by flag", file: plain, theme: "dracula", want: "#282a36"},
		{name: "theme file by flag", file: plain, theme: themeFile, want: "#010203"},
		{name: "unknown flag theme falls back", file: plain, theme: "does-not-exist"},
		{name: "theme from cast header", file: withTheme, want: "#445566"},
		{name: "invalid header theme ignored", file: badTheme},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.svg")
			cmd := newCmd(tt.file, out, "svg")
			cmd.Theme = tt.theme
			cmd.Debug = true
			if err := cmd.Run(); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			data, _ := os.ReadFile(out) //nolint:gosec // test temp file
			if tt.want != "" && !strings.Contains(strings.ToLower(string(data)), tt.want) {
				t.Errorf("output missing %s", tt.want)
			}
		})
	}
}

func TestCmd_Run_Errors(t *testing.T) {
	dir := t.TempDir()
	good := writeRunFile(t, dir, "good.cast", runTestCast)
	bad := writeRunFile(t, dir, "bad.cast", "garbage")

	tests := []struct {
		name  string
		build func() *Cmd
	}{
		{name: "svg option invalid", build: func() *Cmd {
			c := newCmd(good, filepath.Join(dir, "o.svg"), "svg")
			c.SVGLayout = "bogus"
			return c
		}},
		{name: "svg option with gif", build: func() *Cmd {
			c := newCmd(good, filepath.Join(dir, "o.gif"), "gif")
			c.SVGLayout = "bands"
			return c
		}},
		{name: "missing input", build: func() *Cmd {
			return newCmd(filepath.Join(dir, "none.cast"), filepath.Join(dir, "o.svg"), "svg")
		}},
		{name: "unparsable input", build: func() *Cmd { return newCmd(bad, filepath.Join(dir, "o.svg"), "svg") }},
		{name: "unwritable output", build: func() *Cmd { return newCmd(good, filepath.Join(dir, "no-dir", "o.svg"), "svg") }},
		{name: "unsupported format", build: func() *Cmd { return newCmd(good, filepath.Join(dir, "o.xyz"), "xyz") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.build().Run(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestCmd_Run_AsciicastV3MatchesV2(t *testing.T) {
	dir := t.TempDir()
	// The same recording in both formats; v3 stores intervals and may carry
	// comments, markers, resizes and an exit status that must not be drawn.
	v2 := writeRunFile(t, dir, "in.v2.cast", `{"version":2,"width":20,"height":4}
[0.25,"o","hello"]
[1.0,"o","\r\n\u001b[31mred\u001b[0m"]
[1.75,"o","\r\nbye"]
`)
	v3 := writeRunFile(t, dir, "in.v3.cast", `{"version":3,"term":{"cols":20,"rows":4,"type":"xterm-256color"}}
[0.25,"o","hello"]
# a comment between events
[0.5,"m","chapter"]
[0.25,"o","\r\n\u001b[31mred\u001b[0m"]
[0.125,"r","90x30"]
[0.625,"o","\r\nbye"]
[0.5,"x","0"]
`)

	render := func(in string) []byte {
		out := filepath.Join(dir, filepath.Base(in)+".svg")
		if err := newCmd(in, out, "svg").Run(); err != nil {
			t.Fatalf("export %s: %v", filepath.Base(in), err)
		}
		data, err := os.ReadFile(out) //nolint:gosec // test temp file
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if want, got := render(v2), render(v3); !bytes.Equal(want, got) {
		t.Errorf("v3 export differs from the equivalent v2 export (%d vs %d bytes)", len(got), len(want))
	}
}

func TestCmd_Run_RejectsUnusableHeaders(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
	}{
		{name: "v3 without terminal size", content: `{"version":3}` + "\n[1,\"o\",\"x\"]\n"},
		{name: "v2 without terminal size", content: `{"version":2}` + "\n[1,\"o\",\"x\"]\n"},
		{name: "unsupported version", content: `{"version":9,"width":5,"height":2}` + "\n[1,\"o\",\"x\"]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := writeRunFile(t, dir, "bad.cast", tt.content)
			if err := newCmd(in, filepath.Join(dir, "bad.svg"), "svg").Run(); err == nil {
				t.Fatal("expected an error, not a crash")
			}
		})
	}
}

func TestCmd_Run_UsesRecordingIdleTimeLimit(t *testing.T) {
	dir := t.TempDir()
	events := "[0.5,\"o\",\"a\"]\n[10.5,\"o\",\"b\"]\n[20.5,\"o\",\"c\"]\n"
	limited := writeRunFile(t, dir, "limited.cast",
		`{"version":2,"width":20,"height":3,"idle_time_limit":1}`+"\n"+events)
	plain := writeRunFile(t, dir, "plain.cast", `{"version":2,"width":20,"height":3}`+"\n"+events)

	render := func(in string, idle time.Duration) []byte {
		t.Helper()
		out := filepath.Join(dir, filepath.Base(in)+idle.String()+".svg")
		cmd := newCmd(in, out, "svg")
		cmd.MaxIdle = idle
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(out) //nolint:gosec // test temp file
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	t.Run("header limit applies by default", func(t *testing.T) {
		if !bytes.Equal(render(limited, 0), render(plain, time.Second)) {
			t.Error("a recording with idle_time_limit=1 should export like -i 1s")
		}
		if bytes.Equal(render(limited, 0), render(plain, 0)) {
			t.Error("the header limit had no effect")
		}
	})
	t.Run("explicit -i overrides the header", func(t *testing.T) {
		if !bytes.Equal(render(limited, 3*time.Second), render(plain, 3*time.Second)) {
			t.Error("-i 3s should win over the recording's idle_time_limit")
		}
	})
}
