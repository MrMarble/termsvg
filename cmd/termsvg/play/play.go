package play

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mrmarble/termsvg/internal/asciicast"
)

type Cmd struct {
	File    string        `arg:"" type:"existingfile" help:"Asciicast file to play"`
	Speed   float64       `short:"s" default:"1.0" help:"Playback speed multiplier"`
	MaxIdle time.Duration `short:"i" default:"0" help:"Cap idle time between frames (0 = the recording's limit, if any)"`
}

func (cmd *Cmd) Run() error {
	f, err := os.Open(filepath.Clean(cmd.File))
	if err != nil {
		return err
	}
	defer f.Close()

	cast, err := asciicast.Parse(f)
	if err != nil {
		return err
	}

	return playback(cast, cmd.Speed, cast.IdleLimit(cmd.MaxIdle), time.Sleep)
}

// playback prints the output events at their speed-adjusted times, waiting
// with sleep.
func playback(cast *asciicast.Cast, speed float64, maxIdle time.Duration, sleep func(time.Duration)) error {
	cast.ToRelativeTime()

	if maxIdle > 0 {
		cast.CapRelativeTime(maxIdle.Seconds())
	}

	cast.ToAbsoluteTime()
	cast.AdjustSpeed(speed)

	startTime := time.Now()

	for _, event := range cast.Events {
		// Only output is replayed; input, markers, resizes and exit statuses
		// carry data that is not meant for the terminal.
		if event.EventType != asciicast.Output {
			continue
		}

		targetTime := time.Duration(event.Time * float64(time.Second))
		elapsed := time.Since(startTime)

		if delay := targetTime - elapsed; delay > 0 {
			sleep(delay)
		}

		fmt.Print(event.EventData)
	}

	return nil
}
