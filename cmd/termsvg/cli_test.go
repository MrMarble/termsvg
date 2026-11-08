package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type exitSignal struct{}

// runCLI runs the CLI with captured output and records the exit code the
// parser requested, if any.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	exitCode = -1
	exit := func(code int) {
		exitCode = code
		panic(exitSignal{})
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(exitSignal); !ok {
					panic(r)
				}
			}
		}()
		err = run(args, &out, &errOut, exit)
	}()
	return out.String(), errOut.String(), exitCode, err
}

func TestRun_Version(t *testing.T) {
	out, _, code, err := runCLI(t, "--version")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if want := "termsvg dev (none) built on unknown"; !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want containing %q", out, want)
	}
}

func TestRun_Help(t *testing.T) {
	out, _, code, err := runCLI(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	for _, cmd := range []string{"play", "export", "record"} {
		if runtimeHasRecord() || cmd != "record" {
			if !strings.Contains(out, cmd) {
				t.Errorf("help output missing command %q:\n%s", cmd, out)
			}
		}
	}
}

func TestRun_Errors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no command", args: nil},
		{name: "unknown command", args: []string{"frobnicate"}},
		{name: "unknown flag", args: []string{"--nope"}},
		{name: "missing input file", args: []string{"play", filepath.Join(t.TempDir(), "missing.cast")}},
		{name: "invalid enum", args: []string{"export", "-f", "png", "x.cast"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, code, err := runCLI(t, tt.args...)
			if err == nil && code <= 0 {
				t.Errorf("expected an error or non-zero exit, got err=%v code=%d", err, code)
			}
		})
	}
}

func TestRun_ExportSVG(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.cast")
	out := filepath.Join(dir, "out.svg")
	cast := `{"version":2,"width":10,"height":3}` + "\n" + `[0.1,"o","hi"]` + "\n"
	if err := os.WriteFile(in, []byte(cast), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := runCLI(t, "export", "-o", out, in); err != nil {
		t.Fatalf("export failed: %v", err)
	}
	data, err := os.ReadFile(out) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Errorf("output is not an SVG: %.60s", data)
	}
}
