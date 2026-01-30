package record

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/mrmarble/termsvg/pkg/asciicast"
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
	// Create command to run
	c := exec.Command("sh", "-c", cmd.Command) //nolint:gosec // command is from user CLI input

	// Start the command with a PTY
	ptmx, err := pty.Start(c)
	if err != nil {
		return nil, fmt.Errorf("failed to start pty: %w", err)
	}
	defer ptmx.Close()

	// Handle PTY size changes
	ch := handlePtySize(ptmx)
	defer func() {
		signal.Stop(ch)
		close(ch)
	}()

	// Set stdin to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Copy stdin to the PTY with pause support
	var paused atomic.Bool
	go func() {
		buf := make([]byte, readSize)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}

			for i := 0; i < n; i++ {
				// Check for Ctrl+P (0x10) to toggle pause
				if buf[i] == 0x10 {
					paused.Store(!paused.Load())
					continue
				}
				// Write byte to PTY
				_, _ = ptmx.Write(buf[i : i+1])
			}
		}
	}()

	// Read from PTY and record events
	var events []asciicast.Event
	p := make([]byte, readSize)
	baseTime := time.Now().UnixMicro()

	startTriggered := !cmd.SkipFirstLine
	pauseStartTime := int64(0)
	totalPausedTime := int64(0)

	for {
		n, err := ptmx.Read(p)
		if err != nil {
			if err == io.EOF && n > 0 {
				_, _ = os.Stdout.Write(p[:n])
				if !paused.Load() && startTriggered {
					events = append(events, asciicast.Event{
						Time:      float64(time.Now().UnixMicro()-baseTime-totalPausedTime) / float64(time.Millisecond),
						EventType: asciicast.Output,
						EventData: string(p[:n]),
					})
				}
			}
			break
		}

		// Echo to stdout
		_, _ = os.Stdout.Write(p[:n])

		// Handle pause state
		if paused.Load() {
			if pauseStartTime == 0 {
				pauseStartTime = time.Now().UnixMicro()
			}
			continue
		} else if pauseStartTime != 0 {
			totalPausedTime += time.Now().UnixMicro() - pauseStartTime
			pauseStartTime = 0
		}

		// Skip first line if requested
		if !startTriggered {
			if strings.Contains(string(p[:n]), "\n") {
				startTriggered = true
				baseTime = time.Now().UnixMicro()
			}
			continue
		}

		// Record event
		events = append(events, asciicast.Event{
			Time:      float64(time.Now().UnixMicro()-baseTime-totalPausedTime) / float64(time.Millisecond),
			EventType: asciicast.Output,
			EventData: string(p[:n]),
		})
	}

	return events, nil
}

func handlePtySize(ptmx *os.File) chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()

	// Initial resize
	ch <- syscall.SIGWINCH

	return ch
}
