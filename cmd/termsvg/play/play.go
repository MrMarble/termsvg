package play

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
)

type Cmd struct {
	File    string        `arg:"" type:"existingfile" help:"Asciicast file to play"`
	Speed   float64       `short:"s" default:"1.0" help:"Playback speed multiplier"`
	MaxIdle time.Duration `short:"i" default:"0" help:"Cap idle time between frames (0 = unlimited)"`
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

	return playback(cast, cmd.Speed, cmd.MaxIdle)
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
		targetTime := time.Duration(event.Time * float64(time.Second))
		elapsed := time.Since(startTime)

		if delay := targetTime - elapsed; delay > 0 {
			time.Sleep(delay)
		}

		fmt.Print(event.EventData)
	}

	return nil
}
