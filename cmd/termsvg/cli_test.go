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
	for _, cmd := range []string{"play", "export"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("help output missing command %q:\n%s", cmd, out)
		}
	}
	if got := strings.Contains(out, "Record a terminal session"); got != hasRecord {
		t.Errorf("help lists record = %v, want %v:\n%s", got, hasRecord, out)
	}
}

func TestRun_Errors(t *testing.T) {
	cast := filepath.Join(t.TempDir(), "in.cast")
	if err := os.WriteFile(cast, []byte(`{"version":2,"width":10,"height":3}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no command", args: nil, wantErr: "expected one of"},
		{name: "unknown command", args: []string{"frobnicate"}, wantErr: "frobnicate"},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: "--nope"},
		{name: "missing input file", args: []string{"play", filepath.Join(t.TempDir(), "missing.cast")}, wantErr: "missing.cast"},
		{name: "invalid enum", args: []string{"export", "-f", "png", cast}, wantErr: "png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := runCLI(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
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

func TestRun_SystemFontsFlag(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.cast")
	cast := `{"version":2,"width":10,"height":3}` + "\n" + `[0.1,"o","hi"]` + "\n"
	if err := os.WriteFile(in, []byte(cast), 0o600); err != nil {
		t.Fatal(err)
	}

	gifOut := filepath.Join(dir, "a.gif")
	if _, _, _, err := runCLI(t, "export", "--system-fonts", "-f", "gif", "-o", gifOut, in); err != nil {
		t.Errorf("--system-fonts with gif: %v", err)
	}
	if _, _, _, err := runCLI(t, "export", "--system-fonts", "-o", filepath.Join(dir, "a.svg"), in); err == nil {
		t.Error("--system-fonts with the default svg format should be an error")
	}
	if out, _, _, err := runCLI(t, "export", "--help"); err == nil && !strings.Contains(out, "--system-fonts") {
		t.Errorf("--system-fonts is not in the help:\n%s", out)
	}
}
