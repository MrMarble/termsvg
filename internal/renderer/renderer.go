package renderer

import (
	"context"
	"io"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/theme"
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
	ShowCursor bool // Enable cursor rendering (default: true)
	FontFamily string
	FontSize   int
	// SystemFonts lets the raster formats (GIF, WebM) draw glyphs that the
	// embedded fonts lack with installed fonts. Output then depends on the machine.
	SystemFonts bool
	LoopCount   int // 0 = infinite, -1 = no loop
	Minify      bool
	Debug       bool // Enable debug logging

	// Video encoding options (for WebM/MP4 formats)
	VideoBitrate int // Video bitrate in kbps (0 = use default)
	FrameRate    int // Target frame rate in FPS (0 = auto-calculate)

	// ProgressCh is an optional channel for progress updates
	ProgressCh chan<- progress.Update
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Theme:      theme.Default(),
		ShowWindow: true,
		ShowCursor: true,
		FontFamily: "Monaco,Consolas,'Courier New',monospace",
		FontSize:   20,
		LoopCount:  0,
		Minify:     false,
	}
}
