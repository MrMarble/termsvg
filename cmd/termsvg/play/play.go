package play

import (
	"fmt"
	"time"

	"github.com/mrmarble/termsvg/cmd/termsvg/internal/pacing"
	"github.com/mrmarble/termsvg/internal/asciicast"
)

type Cmd struct {
	File string `arg:"" type:"existingfile" help:"Asciicast file to play"`

	pacing.Flags `embed:""`
}

func (cmd *Cmd) Run() error {
	cast, err := asciicast.ParseFile(cmd.File)
	if err != nil {
		return err
	}

	return playback(cast, cmd.Speed, cast.IdleLimit(cmd.MaxIdle))
}

func playback(cast *asciicast.Cast, speed float64, maxIdle time.Duration) error {
	// Convert to relative time for idle capping
	cast.ToRelativeTime()

	// Cap idle time if specified
	if maxIdle > 0 {
		cast.CapRelativeTime(maxIdle.Seconds())
	}

	// Convert back to absolute and adjust speed
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
			time.Sleep(delay)
		}

		fmt.Print(event.EventData)
	}

	return nil
}
