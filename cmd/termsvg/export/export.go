package export

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
	"github.com/mrmarble/termsvg/pkg/renderer/svg"
	"github.com/tdewolff/minify/v2"
	msvg "github.com/tdewolff/minify/v2/svg"
)

type Cmd struct {
	File     string        `arg:"" type:"existingfile" help:"Asciicast file to export"`
	Output   string        `short:"o" type:"path" help:"Output file path (default: <input>.svg)"`
	Minify   bool          `short:"m" help:"Minify output SVG"`
	NoWindow bool          `short:"n" help:"Don't render terminal window chrome"`
	Speed    float64       `short:"s" default:"1.0" help:"Playback speed multiplier"`
	MaxIdle  time.Duration `short:"i" default:"0" help:"Cap idle time between frames (0 = unlimited)"`
	Cols     int           `short:"c" default:"0" help:"Override columns (0 = use original)"`
	Rows     int           `short:"r" default:"0" help:"Override rows (0 = use original)"`
}

func (cmd *Cmd) Run() error {
	output := cmd.Output
	if output == "" {
		output = cmd.File + ".svg"
	}

	// Load cast file
	f, err := os.Open(filepath.Clean(cmd.File))
	if err != nil {
		return err
	}
	defer f.Close()

	cast, err := asciicast.Parse(f)
	if err != nil {
		return err
	}

	// Override dimensions if specified
	if cmd.Cols > 0 {
		cast.Header.Width = cmd.Cols
	}
	if cmd.Rows > 0 {
		cast.Header.Height = cmd.Rows
	}

	// Process through IR
	procConfig := ir.DefaultProcessorConfig()
	procConfig.Speed = cmd.Speed
	procConfig.IdleTimeLimit = cmd.MaxIdle

	proc := ir.NewProcessor(procConfig)
	rec, err := proc.Process(cast)
	if err != nil {
		return err
	}

	// Render to SVG
	renderConfig := renderer.DefaultConfig()
	renderConfig.ShowWindow = !cmd.NoWindow

	svgRenderer := svg.New(renderConfig)

	// Create output file
	outFile, err := os.Create(output)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Render (with optional minification)
	if cmd.Minify {
		var buf bytes.Buffer
		if err := svgRenderer.Render(context.Background(), rec, &buf); err != nil {
			return err
		}
		m := minify.New()
		m.AddFunc("image/svg+xml", msvg.Minify)
		if err := m.Minify("image/svg+xml", outFile, &buf); err != nil {
			return err
		}
	} else {
		if err := svgRenderer.Render(context.Background(), rec, outFile); err != nil {
			return err
		}
	}

	fmt.Printf("Exported: %s\n", output)
	return nil
}
