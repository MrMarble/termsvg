package play

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
)

type Cmd struct {
	File    string  `arg:"" type:"existingfile" help:"termsvg recording file"`
	Speed   float64 `optional:"" short:"s" default:"1.0" help:"Playback speed (can be fractional)"`
	IdleCap float64 `optional:"" short:"i" default:"-1.0" help:"Limit replayed terminal inactivity to max seconds. (-1 for unlimited)"` //nolint
}

func (cmd *Cmd) Run() error {
	return play(cmd.File, cmd.IdleCap, cmd.Speed)
}

func play(path string, idleCap, speed float64) error {
	file, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}

	records, err := asciicast.Unmarshal(file)
	if err != nil {
		return err
	}

	records.ToRelativeTime()
	records.CapRelativeTime(idleCap)
	records.ToAbsoluteTime()
	records.AdjustSpeed(speed)

	baseTime := time.Duration(time.Now().UnixMilli()) * time.Millisecond

	for _, record := range records.Events {
		duration := time.Duration(record.Time * float64(time.Second))

		delay := duration - ((time.Duration(time.Now().UnixMilli()) * time.Millisecond) - baseTime)

		time.Sleep(delay)
		fmt.Print(record.EventData)
	}

	return nil
}
