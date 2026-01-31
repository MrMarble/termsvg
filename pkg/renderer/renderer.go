package renderer

import (
	"context"
	"io"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/theme"
)

// Renderer defines the interface for output formats
type Renderer interface {
	Render(ctx context.Context, rec *ir.Recording, w io.Writer) error
	Format() string
	FileExtension() string
}

// Config holds renderer options
type Config struct {
	Theme      theme.Theme
	ShowWindow bool
	FontFamily string
	FontSize   int
	LoopCount  int // 0 = infinite, -1 = no loop
	Minify     bool
	Debug      bool // Enable debug logging

	// Video encoding options (for WebM/MP4 formats)
	VideoBitrate int // Video bitrate in kbps (0 = use default)
	FrameRate    int // Target frame rate in FPS (0 = auto-calculate)
}

func DefaultConfig() Config {
	return Config{
		Theme:      theme.Default(),
		ShowWindow: true,
		FontFamily: "Monaco,Consolas,'Courier New',monospace",
		FontSize:   20,
		LoopCount:  0,
		Minify:     false,
	}
}
