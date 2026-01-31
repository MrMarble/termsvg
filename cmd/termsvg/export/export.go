package export

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
	"github.com/mrmarble/termsvg/pkg/renderer/gif"
	"github.com/mrmarble/termsvg/pkg/renderer/svg"
	"github.com/mrmarble/termsvg/pkg/renderer/webm"
	"github.com/mrmarble/termsvg/pkg/theme"
	"github.com/tdewolff/minify/v2"
	msvg "github.com/tdewolff/minify/v2/svg"
)

type Cmd struct {
	File     string        `arg:"" type:"existingfile" help:"Asciicast file to export"`
	Output   string        `short:"o" type:"path" help:"Output file path (default: <input>.<format>)"`
	Format   string        `short:"f" default:"svg" enum:"svg,gif,webm" help:"Output format (svg, gif, webm)"`
	Minify   bool          `short:"m" help:"Minify output (SVG only)"`
	NoWindow bool          `short:"n" help:"Don't render terminal window chrome"`
	Speed    float64       `short:"s" default:"1.0" help:"Playback speed multiplier"`
	MaxIdle  time.Duration `short:"i" default:"0" help:"Cap idle time between frames (0 = unlimited)"`
	Cols     int           `short:"c" default:"0" help:"Override columns (0 = use original)"`
	Rows     int           `short:"r" default:"0" help:"Override rows (0 = use original)"`
	Debug    bool          `short:"d" help:"Enable debug logging"`
	Theme    string        `short:"t" help:"Theme name (built-in) or path to theme JSON file"`
}

//nolint:funlen,gocognit // sequential export steps are clearer in one function
func (cmd *Cmd) Run() error {
	format := strings.ToLower(cmd.Format)

	output := cmd.Output
	if output == "" {
		output = cmd.File + "." + format
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

	// Determine theme to use
	selectedTheme := theme.Default()
	themeSource := "default"

	if cmd.Theme != "" {
		// CLI flag takes priority
		t, err := theme.Load(cmd.Theme)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load theme %q: %v\n", cmd.Theme, err)
			fmt.Fprintf(os.Stderr, "Falling back to default theme\n")
		} else {
			selectedTheme = t
			themeSource = "CLI flag"
		}
	} else if cast.Header.Theme.Fg != "" {
		// Use theme from asciicast header
		t, err := theme.FromAsciinema("asciicast", cast.Header.Theme.Fg,
			cast.Header.Theme.Bg, cast.Header.Theme.Palette)
		if err != nil {
			if cmd.Debug {
				log.Printf("[Export] Invalid theme in asciicast header: %v", err)
			}
		} else {
			selectedTheme = t
			themeSource = "asciicast header"
		}
	}

	if cmd.Debug {
		log.Printf("[Export] Using theme from %s: %s", themeSource, selectedTheme.Name)
	}

	// Process through IR
	procConfig := ir.DefaultProcessorConfig()
	procConfig.Speed = cmd.Speed
	procConfig.IdleTimeLimit = cmd.MaxIdle
	procConfig.Theme = selectedTheme

	proc := ir.NewProcessor(procConfig)
	rec, err := proc.Process(cast)
	if err != nil {
		return err
	}

	// Create renderer based on format
	renderConfig := renderer.DefaultConfig()
	renderConfig.ShowWindow = !cmd.NoWindow
	renderConfig.Minify = cmd.Minify
	renderConfig.Debug = cmd.Debug
	renderConfig.Theme = selectedTheme

	var rdr renderer.Renderer
	switch format {
	case "gif":
		gifRenderer, err := gif.New(renderConfig)
		if err != nil {
			return fmt.Errorf("failed to create GIF renderer: %w", err)
		}
		rdr = gifRenderer
	case "svg":
		rdr = svg.New(renderConfig)
	case "webm":
		webmRenderer, err := webm.New(renderConfig)
		if err != nil {
			return fmt.Errorf("failed to create WebM renderer: %w", err)
		}
		rdr = webmRenderer
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	// Create output file
	outFile, err := os.Create(output) //nolint:gosec // output path is from user CLI input
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Render (with optional minification for SVG)
	if cmd.Minify && format == "svg" {
		var buf bytes.Buffer
		if err := rdr.Render(context.Background(), rec, &buf); err != nil {
			return err
		}
		m := minify.New()
		m.AddFunc("image/svg+xml", msvg.Minify)
		var minified bytes.Buffer
		if err := m.Minify("image/svg+xml", &minified, &buf); err != nil {
			return err
		}
		// Replace non-breaking spaces back to regular spaces after minification
		result := strings.ReplaceAll(minified.String(), "\u00A0", " ")
		if _, err := outFile.WriteString(result); err != nil {
			return err
		}
	} else {
		if err := rdr.Render(context.Background(), rec, outFile); err != nil {
			return err
		}
	}

	fmt.Printf("Exported: %s\n", output)
	return nil
}
