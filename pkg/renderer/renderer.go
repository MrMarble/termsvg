package renderer

import (
	"context"
	"fmt"
	"io"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/progress"
	"github.com/mrmarble/termsvg/pkg/raster"
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
	ShowCursor bool // Enable cursor rendering (default: true)
	FontFamily string
	FontSize   int
	LoopCount  int // 0 = infinite, -1 = no loop
	Minify     bool
	Debug      bool // Enable debug logging

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

// NewRasterizer creates a raster.Rasterizer from renderer configuration.
// This helper reduces duplication between renderers that need rasterization.
func NewRasterizer(config *Config) (*raster.Rasterizer, error) {
	rasterConfig := raster.Config{
		Theme:      config.Theme,
		ShowWindow: config.ShowWindow,
		ShowCursor: config.ShowCursor,
		FontSize:   config.FontSize,
		RowHeight:  raster.RowHeight,
		ColWidth:   raster.ColWidth,
		Padding:    raster.Padding,
		HeaderSize: raster.HeaderSize,
		ProgressCh: config.ProgressCh,
	}

	rasterizer, err := raster.New(&rasterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rasterizer: %w", err)
	}

	return rasterizer, nil
}
