//go:build !windows

// Package record implements the record command, which runs a program in a
// pseudo-terminal and saves its output as an asciicast. Recording needs a PTY,
// so the package is only built on non-Windows platforms.
package record

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/mrmarble/termsvg/internal/asciicast"
	"golang.org/x/term"
)

type Cmd struct {
	File          string `arg:"" type:"path" help:"Filename/path to save the recording to"`
	Command       string `short:"c" optional:"" env:"SHELL" help:"Command to record (default: $SHELL)"`
	SkipFirstLine bool   `short:"s" help:"Skip the first line of recording"`
}

const readSize = 1024

func (cmd *Cmd) Run() error {
	fmt.Printf("Recording to %s\n", cmd.File)
	fmt.Println("Press Ctrl+D or type 'exit' to stop recording.")
	fmt.Println("Press Ctrl+P to pause/resume recording.")

	if cmd.SkipFirstLine {
		fmt.Println("Note: Skipping the first line of output.")
	}

	events, err := cmd.run()
	if err != nil {
		return err
	}

	if err := cmd.save(events); err != nil {
		return err
	}

	fmt.Printf("Recording saved: %s\n", cmd.File)
	return nil
}

func (cmd *Cmd) save(events []asciicast.Event) error {
	if len(events) == 0 {
		return fmt.Errorf("no events recorded")
	}

	cast := asciicast.New()

	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	cast.Header.Width = width
	cast.Header.Height = height
	cast.Header.Duration = events[len(events)-1].Time
	cast.Events = events
	cast.Compress()

	data, err := cast.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal cast: %w", err)
	}

	if err := os.WriteFile(cmd.File, data, 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

//nolint:gocognit,funlen // PTY handling requires sequential state management
func (cmd *Cmd) run() ([]asciicast.Event, error) {
	c := exec.Command("sh", "-c", cmd.Command) //nolint:gosec // command is from user CLI input

	ptmx, err := pty.Start(c)
	if err != nil {
		return nil, fmt.Errorf("failed to start pty: %w", err)
	}
	defer ptmx.Close()

	stdin := os.Stdin

	// Handle PTY size changes. stopResize waits for the resize goroutine, so it
	// must run before ptmx is closed (defers run last-in first-out).
	stopResize := handlePtySize(stdin, ptmx)
	defer stopResize()

	oldState, err := term.MakeRaw(int(stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(stdin.Fd()), oldState)

	var paused atomic.Bool
	go func() {
		buf := make([]byte, readSize)
		for {
			n, err := stdin.Read(buf)
			if err != nil {
				return
			}

			for i := range n {
				// Check for Ctrl+P (0x10) to toggle pause
				if buf[i] == 0x10 {
					paused.Store(!paused.Load())
					continue
				}
				_, _ = ptmx.Write(buf[i : i+1])
			}
		}
	}()

	events := make([]asciicast.Event, 0, 1024)
	p := make([]byte, readSize)
	baseTime := time.Now().UnixMicro()

	startTriggered := !cmd.SkipFirstLine
	var pauseStartTime, totalPausedTime int64

	output := func(data []byte) asciicast.Event {
		return asciicast.Event{
			Time:      float64(time.Now().UnixMicro()-baseTime-totalPausedTime) / float64(time.Millisecond),
			EventType: asciicast.Output,
			EventData: string(data),
		}
	}

	for {
		n, err := ptmx.Read(p)
		if err != nil {
			if err == io.EOF && n > 0 {
				_, _ = os.Stdout.Write(p[:n])
				if !paused.Load() && startTriggered {
					events = append(events, output(p[:n]))
				}
			}
			break
		}

		_, _ = os.Stdout.Write(p[:n])

		if paused.Load() {
			if pauseStartTime == 0 {
				pauseStartTime = time.Now().UnixMicro()
			}
			continue
		} else if pauseStartTime != 0 {
			totalPausedTime += time.Now().UnixMicro() - pauseStartTime
			pauseStartTime = 0
		}

		// Skip first line if requested: drop everything up to and including the
		// first newline, and record whatever follows it in the same read.
		chunk := p[:n]
		if !startTriggered {
			_, rest, found := bytes.Cut(chunk, []byte("\n"))
			if !found {
				continue
			}
			startTriggered = true
			baseTime = time.Now().UnixMicro()
			if len(rest) == 0 {
				continue
			}
			chunk = rest
		}

		events = append(events, output(chunk))
	}

	return events, nil
}

// handlePtySize keeps the PTY size in sync with stdin on SIGWINCH. The returned
// function stops the watcher and waits for it to exit, so the caller can safely
// close ptmx afterwards.
func handlePtySize(stdin, ptmx *os.File) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
			_ = pty.InheritSize(stdin, ptmx)
		}
	}()

	ch <- syscall.SIGWINCH

	return func() {
		signal.Stop(ch)
		close(ch)
		<-done
	}
}
