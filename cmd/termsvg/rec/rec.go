package rec

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
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
	log.Info().Str("output", cmd.File).Msg("recording asciicast.")
	log.Info().Msg("exit the opened program when you're done.")

	err := rec(cmd.File, cmd.Command)
	if err != nil {
		return err
	}

	log.Info().Msg("recording finished.")
	log.Info().Str("output", cmd.File).Msg("asciicast saved.")

	return nil
}

func rec(file, command string) error {
	events, err := run(command)
	if err != nil {
		return err
	}

	rec := asciicast.New()

	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return err
	}

	rec.Header.Width = width
	rec.Header.Height = height
	rec.Header.Duration = events[len(events)-1].Time
	rec.Events = events
	rec.Compress()

	js, err := rec.Marshal()
	if err != nil {
		return err
	}

	err = os.WriteFile(file, js, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

//nolint
func run(command string) ([]asciicast.Event, error) {
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
			log.Fatal().Err(err).Msg("error closing pty")
		}
	}() // Best effort.

	ch := handlePtySize(ptmx)
	defer func() { signal.Stop(ch); close(ch) }() // Cleanup signals when done.

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatal().Err(err).Msg("error setting stin in raw mode")
	}

	defer func() {
		if err = term.Restore(int(os.Stdin.Fd()), oldState); err != nil {
			log.Fatal().Err(err).Msg("error restoring terminal")
		}
	}() // Best effort.

	// Copy stdin to the pty and the pty to stdout.
	// NOTE: The goroutine will keep reading until the next keystroke before returning.
	go func() {
		if _, err = io.Copy(ptmx, os.Stdin); err != nil {
			log.Fatal().Err(err).Msg("error reading stdin")
		}
	}()

	var events []asciicast.Event

	p := make([]byte, readSize)
	baseTime := time.Now().UnixMicro()

	for {
		n, err := ptmx.Read(p)
		event := asciicast.Event{
			Time:      float64(time.Now().UnixMicro()-baseTime) / float64(time.Millisecond),
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

	return events, nil
}

func handlePtySize(ptmx *os.File) chan os.Signal {
	// Handle pty size.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Fatal().Err(err).Msg("error resizing pty")
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	return ch
}
