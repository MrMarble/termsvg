package renderer

import (
	"context"
	"fmt"
	"io"

	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/raster"
	"github.com/mrmarble/termsvg/internal/theme"
)

// Renderer writes a recording in one output format.
type Renderer interface {
	Render(ctx context.Context, rec *ir.Recording, w io.Writer) error
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

	VideoBitrate int // Video bitrate cap in kbps for WebM (0 = no cap)

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

// NewRasterizer creates the raster.Rasterizer the raster formats (GIF, WebM) draw with.
func NewRasterizer(config *Config) (*raster.Rasterizer, error) {
	rasterConfig := raster.Config{
		Theme:       config.Theme,
		ShowWindow:  config.ShowWindow,
		ShowCursor:  config.ShowCursor,
		FontSize:    config.FontSize,
		SystemFonts: config.SystemFonts,
		ProgressCh:  config.ProgressCh,
	}

	rasterizer, err := raster.New(&rasterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rasterizer: %w", err)
	}

	return rasterizer, nil
}
