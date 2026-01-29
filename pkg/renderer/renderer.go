package renderer

import (
	"context"
	"io"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/theme"
)

// Renderer defines the interface for output formats
type Renderer interface {
	Render(ctx context.Context, cast *asciicast.Cast, w io.Writer) error
	Format() string
	FileExtension() string
}

// Config holds renderer options
type Config struct {
	Theme      theme.Theme
	ShowWindow bool
	FontFamily string
	FontSize   int

	Speed         float64 // Playback speed multiplier
	IdleTimeLimit float64 // Cap idle time (-1 = unlimited)
	LoopCount     int     // 0 = infinite, -1 = no loop

	Minify bool
}

func DefaultConfig() Config {
	return Config{
		Theme:         theme.Default(),
		ShowWindow:    true,
		FontFamily:    "Monaco,Consolas,'Courier New',monospace",
		FontSize:      20,
		Speed:         1.0,
		IdleTimeLimit: -1,
		LoopCount:     0,
		Minify:        false,
	}
}
