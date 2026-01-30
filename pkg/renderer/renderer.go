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
