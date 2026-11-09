package export

import (
	"bytes"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
[0.125,"r","20x4"]
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

func TestCmd_Run_EightColorHeaderTheme(t *testing.T) {
	dir := t.TempDir()
	in := writeRunFile(t, dir, "eight.cast",
		`{"version":3,"term":{"cols":20,"rows":3,"theme":{"fg":"#112233","bg":"#445566",`+
			`"palette":"#000000:#cc0000:#00cc00:#cccc00:#0000cc:#cc00cc:#00cccc:#cccccc"}}}`+"\n"+
			"[0.1,\"o\",\"\\u001b[31mred\\u001b[0m \\u001b[91mbright\\u001b[0m\"]\n")
	out := filepath.Join(dir, "eight.svg")
	if err := newCmd(in, out, "svg").Run(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	svg := strings.ToLower(string(data))
	if !strings.Contains(svg, "#445566") {
		t.Error("the header theme background was not applied")
	}
	// Normal red and bright red both map to the theme's red (#cc0000), because
	// an 8-color palette repeats its colors for the bright half.
	if !strings.Contains(svg, "#c00") && !strings.Contains(svg, "#cc0000") {
		t.Error("themed red missing from output")
	}
	if strings.Contains(svg, "#ff0000") || strings.Contains(svg, "#f00") {
		t.Error("the default bright red was used instead of the theme's repeated red")
	}
}

func TestCmd_Run_ResizeGrowsTheCanvas(t *testing.T) {
	dir := t.TempDir()
	fixed := writeRunFile(t, dir, "fixed.cast", `{"version":3,"term":{"cols":20,"rows":4}}
[0.25,"o","hello"]
[0.25,"o","\r\nworld"]
`)
	resized := writeRunFile(t, dir, "resized.cast", `{"version":3,"term":{"cols":20,"rows":4}}
[0.25,"o","hello"]
[0.125,"r","40x8"]
[0.125,"o","\r\nworld"]
`)

	widthOf := func(in string) int {
		t.Helper()
		out := filepath.Join(dir, filepath.Base(in)+".svg")
		cmd := newCmd(in, out, "svg")
		cmd.NoWindow = true
		if err := cmd.Run(); err != nil {
			t.Fatalf("export %s: %v", filepath.Base(in), err)
		}
		data, err := os.ReadFile(out) //nolint:gosec // test temp file
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`<svg[^>]*\swidth="(\d+)"`).FindSubmatch(data)
		if match == nil {
			t.Fatalf("no width in %.120s", data)
		}
		width, err := strconv.Atoi(string(match[1]))
		if err != nil {
			t.Fatal(err)
		}
		return width
	}

	if fixedWidth, resizedWidth := widthOf(fixed), widthOf(resized); resizedWidth <= fixedWidth {
		t.Errorf("canvas width after growing to 40 columns = %d, want more than the fixed %d", resizedWidth, fixedWidth)
	}
}

func TestCmd_Run_SystemFontsOnlyApplyToRasterFormats(t *testing.T) {
	dir := t.TempDir()
	in := writeRunFile(t, dir, "in.cast", runTestCast)
	// Keep the font search small and local: the flag must work whatever is installed.
	t.Setenv("TERMSVG_FONT_PATH", dir)

	t.Run("svg is rejected with an explanation", func(t *testing.T) {
		cmd := newCmd(in, filepath.Join(dir, "out.svg"), "svg")
		cmd.SystemFonts = true
		err := cmd.Run()
		if err == nil || !strings.Contains(err.Error(), "--system-fonts") || !strings.Contains(err.Error(), "gif and webm") {
			t.Fatalf("error = %v, want one that explains --system-fonts is for gif and webm", err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "out.svg")); statErr == nil {
			t.Error("an SVG was written although the options were invalid")
		}
	})

	t.Run("gif accepts it", func(t *testing.T) {
		out := filepath.Join(dir, "out.gif")
		cmd := newCmd(in, out, "gif")
		cmd.SystemFonts = true
		if err := cmd.Run(); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, err := os.ReadFile(out) //nolint:gosec // test temp file
		if err != nil || !bytes.HasPrefix(data, []byte("GIF8")) {
			t.Fatalf("output is not a GIF: %v %.8q", err, data)
		}
	})

	t.Run("it changes nothing for text the embedded fonts cover", func(t *testing.T) {
		export := func(systemFonts bool) []byte {
			out := filepath.Join(dir, "cmp.gif")
			cmd := newCmd(in, out, "gif")
			cmd.SystemFonts = systemFonts
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(out) //nolint:gosec // test temp file
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
		if !bytes.Equal(export(false), export(true)) {
			t.Error("--system-fonts changed the output of a recording the embedded fonts fully cover")
		}
	})
}

func TestCmd_Run_GIFShowsEachStateWhenItsEventHappens(t *testing.T) {
	dir := t.TempDir()
	// A appears at 0.5s, B at 3.5s, C at 5.5s. Each stays on screen until the
	// next appears; the last one is held a little so it can be read.
	in := writeRunFile(t, dir, "timing.cast", `{"version":2,"width":12,"height":2}
[0.5,"o","A"]
[3.5,"o","B"]
[5.5,"o","C"]
`)
	out := filepath.Join(dir, "timing.gif")
	if err := newCmd(in, out, "gif").Run(); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(out) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	g, err := gif.DecodeAll(file)
	if err != nil {
		t.Fatal(err)
	}

	// Delays are in hundredths of a second.
	if want := []int{300, 200, 200}; len(g.Delay) != len(want) || g.Delay[0] != want[0] || g.Delay[1] != want[1] ||
		g.Delay[2] < want[2] {
		t.Errorf("GIF frame delays = %v, want A shown for 3.00s, AB for 2.00s and ABC held (>= 2.00s)", g.Delay)
	}
}

func TestCmd_Run_SVGAndGIFAgreeWhenNothingVisibleChanges(t *testing.T) {
	dir := t.TempDir()
	// Events 2 and 4 change nothing. A must still be on screen from 0.5s until B
	// appears at 4.0s, and the recording must still end at 6.0s.
	in := writeRunFile(t, dir, "dup.cast", `{"version":2,"width":12,"height":2}
[0.5,"o","A"]
[2.0,"o","\u001b[0m"]
[4.0,"o","B"]
[6.0,"o","\u001b[0m"]
`)
	out := filepath.Join(dir, "dup.gif")
	if err := newCmd(in, out, "gif").Run(); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(out) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	g, err := gif.DecodeAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Delay) != 2 || g.Delay[0] != 350 || g.Delay[1] != 200 {
		t.Errorf("GIF frame delays = %v, want [350 200]: A for 3.50s, then AB for the remaining 2.00s", g.Delay)
	}
}
