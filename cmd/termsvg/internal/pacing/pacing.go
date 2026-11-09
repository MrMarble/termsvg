// Package pacing holds the playback timing flags shared by play and export.
package pacing

import "time"

// Flags are the timing flags of commands that replay a recording.
type Flags struct {
	Speed   float64       `short:"s" default:"1.0" help:"Playback speed multiplier"`
	MaxIdle time.Duration `short:"i" default:"0" help:"Cap idle time (0 = recording's idle_time_limit, if any)"`
}
