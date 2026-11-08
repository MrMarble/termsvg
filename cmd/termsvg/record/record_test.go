package record

import (
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/mrmarble/termsvg/internal/asciicast"
)

func TestCmd_Save_NoEvents(t *testing.T) {
	cmd := &Cmd{File: filepath.Join(t.TempDir(), "out.cast")}
	if err := cmd.save(nil); err == nil {
		t.Fatal("expected error for empty recording")
	}
	if _, err := os.Stat(cmd.File); !os.IsNotExist(err) {
		t.Error("file should not be created when there are no events")
	}
}

func TestCmd_Save_RequiresTerminal(t *testing.T) {
	// Under `go test` stdout is not a TTY, so the size lookup must fail
	// cleanly instead of writing a cast with a zero-sized header.
	cmd := &Cmd{File: filepath.Join(t.TempDir(), "out.cast")}
	events := []asciicast.Event{{Time: 1, EventType: asciicast.Output, EventData: "x"}}
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		t.Skip("stdout is a terminal")
	}
	if err := cmd.save(events); err == nil {
		t.Fatal("expected terminal size error")
	}
}

func TestCmd_Run_NoTTYStdin(t *testing.T) {
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		t.Skip("stdin is a terminal")
	}
	cmd := &Cmd{File: filepath.Join(t.TempDir(), "out.cast"), Command: "true"}
	if _, err := cmd.run(); err == nil {
		t.Fatal("expected raw-mode error without a TTY stdin")
	}
}

func TestHandlePtySize(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("cannot open pty: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	ch := handlePtySize(ptmx)
	if ch == nil {
		t.Fatal("expected a signal channel")
	}
	signal.Stop(ch)
	close(ch)
}

// withPtyStdio replaces os.Stdin/os.Stdout with a pty slave for the test and
// drains the master side so the recorded program never blocks on output.
func withPtyStdio(t *testing.T) {
	t.Helper()
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("cannot open pty: %v", err)
	}
	if err := pty.Setsize(tty, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	origIn, origOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = tty, tty
	go func() { _, _ = io.Copy(io.Discard, ptmx) }()
	t.Cleanup(func() {
		os.Stdin, os.Stdout = origIn, origOut
		_ = tty.Close()
		_ = ptmx.Close()
	})
}

func TestCmd_Run_RecordsSession(t *testing.T) {
	withPtyStdio(t)
	file := filepath.Join(t.TempDir(), "out.cast")
	cmd := &Cmd{File: file, Command: "printf hello"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	f, err := os.Open(file) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cast, err := asciicast.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if cast.Header.Width != 80 || cast.Header.Height != 24 {
		t.Errorf("size = %dx%d, want 80x24", cast.Header.Width, cast.Header.Height)
	}
	var out strings.Builder
	for _, e := range cast.Events {
		out.WriteString(e.EventData)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("recorded output = %q", out.String())
	}
}

func TestCmd_Run_SkipFirstLine(t *testing.T) {
	withPtyStdio(t)
	file := filepath.Join(t.TempDir(), "out.cast")
	cmd := &Cmd{File: file, Command: "printf 'first\\n'; sleep 0.3; printf second", SkipFirstLine: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(file) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "first") || !strings.Contains(string(data), "second") {
		t.Errorf("unexpected cast content: %s", data)
	}
}

func TestCmd_Run_NoOutputIsError(t *testing.T) {
	withPtyStdio(t)
	cmd := &Cmd{File: filepath.Join(t.TempDir(), "out.cast"), Command: "true"}
	if err := cmd.Run(); err == nil {
		t.Fatal("expected error when nothing was recorded")
	}
}

func TestCmd_Save_WriteError(t *testing.T) {
	withPtyStdio(t)
	cmd := &Cmd{File: filepath.Join(t.TempDir(), "missing-dir", "out.cast")}
	events := []asciicast.Event{{Time: 1, EventType: asciicast.Output, EventData: "x"}}
	if err := cmd.save(events); err == nil {
		t.Fatal("expected write error")
	}
}
