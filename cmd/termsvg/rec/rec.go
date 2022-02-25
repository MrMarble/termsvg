package rec

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/fatih/color"
	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/rs/zerolog/log"
	"golang.org/x/term"
)

type Cmd struct {
	File    string `arg:"" type:"path" help:"filename/path to save the recording to"`
	Command string `short:"c" optional:"" env:"SHELL" help:"Specify command to record, defaults to $SHELL"`
}

const readSize = 1024

func (cmd *Cmd) Run() error {
	color.Green("recording asciicast to %s", cmd.File)
	color.Green("exit the opened program when you're done")

	events, err := run(cmd.Command)
	if err != nil {
		return err
	}

	rec := asciicast.NewRecord()

	rec.Events = events

	js, err := rec.ToJSON()
	if err != nil {
		return err
	}

	color.Green("recording finished")

	err = os.WriteFile(cmd.File, js, os.ModePerm)
	if err != nil {
		return err
	}

	color.Green("asciicast saved to %s", cmd.File)

	return nil
}

func newPty(command string) (*os.File, error) {
	// Create arbitrary command.
	c := exec.Command("sh", "-c", command)
	// Start the command with a pty.
	ptmx, err := pty.Start(c)
	if err != nil {
		return nil, err
	}
	// Make sure to close the pty at the end.
	defer func() {
		if err = ptmx.Close(); err != nil {
			log.Printf("error closing pty: %s", err)
		}
	}() // Best effort.

	// Handle pty size.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		for range ch {
			if err = pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Printf("error resizing pty: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	defer func() { signal.Stop(ch); close(ch) }() // Cleanup signals when done.

	return ptmx, err
}

func run(command string) ([]asciicast.Event, error) {
	ptmx, err := newPty(command)
	if err != nil {
		return nil, err
	}

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}

	defer func() {
		if err = term.Restore(int(os.Stdin.Fd()), oldState); err != nil {
			log.Printf("error restoring terminal: %s", err)
		}
	}() // Best effort.

	// Copy stdin to the pty and the pty to stdout.
	// NOTE: The goroutine will keep reading until the next keystroke before returning.
	go func() {
		if _, err = io.Copy(ptmx, os.Stdin); err != nil {
			log.Printf("error reading stdin: %s", err)
		}
	}()

	var events []asciicast.Event

	p := make([]byte, readSize)
	baseTime := time.Now().UnixMilli()

	for {
		n, err := ptmx.Read(p)
		event := asciicast.Event{
			Time:      float64(time.Now().UnixMilli()-baseTime) / float64(time.Microsecond),
			EventType: asciicast.Output, EventData: string(p[:n]),
		}

		if err != nil {
			if err == io.EOF {
				os.Stdout.Write(p[:n]) // should handle any remainding bytes.

				events = append(events, event)
			}

			break
		}

		os.Stdout.Write(p[:n])

		events = append(events, event)
	}

	//_, _ = io.Copy(output, ptmx)

	// os.WriteFile("output.test", buff.Bytes(), 0644)
	return events, nil
}
