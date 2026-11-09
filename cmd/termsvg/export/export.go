//nolint:lll // Kong option tags cannot be split across lines.
package export

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/internal/asciicast"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/renderer"
	"github.com/mrmarble/termsvg/internal/renderer/gif"
	"github.com/mrmarble/termsvg/internal/renderer/svg"
	"github.com/mrmarble/termsvg/internal/renderer/webm"
	"github.com/mrmarble/termsvg/internal/svgoutput"
	"github.com/mrmarble/termsvg/internal/theme"
)

type Cmd struct {
	File             string        `arg:"" type:"existingfile" help:"Asciicast file to export"`
	Output           string        `short:"o" type:"path" help:"Output file path (default: <input>.<format>)"`
	Format           string        `short:"f" default:"svg" enum:"svg,gif,webm" help:"Output format (svg, gif, webm)"`
	Minify           bool          `short:"m" help:"Minify output (SVG only)"`
	NoWindow         bool          `short:"n" help:"Don't render terminal window chrome"`
	NoCursor         bool          `short:"C" help:"Don't render cursor"`
	Speed            float64       `short:"s" default:"1.0" help:"Playback speed multiplier"`
	MaxIdle          time.Duration `short:"i" default:"0" help:"Cap idle time between frames (0 = unlimited)"`
	Cols             int           `short:"c" default:"0" help:"Override columns (0 = use original)"`
	Rows             int           `short:"r" default:"0" help:"Override rows (0 = use original)"`
	Debug            bool          `short:"d" help:"Enable debug logging"`
	Theme            string        `short:"t" help:"Theme name (built-in) or path to theme JSON file"`
	SVGLayout        string        `default:"frames" enum:"frames,bands,regions,scroll,auto" help:"SVG layout: frames, experimental bands/regions/scroll, or measured auto selection"`
	SVGAnimation     string        `default:"css" enum:"css,smil" help:"SVG animation backend (css or experimental smil)"`
	SVGFrameSwitch   string        `default:"translate" enum:"translate,href" help:"SVG state switching: translated strips or experimental animated hrefs"`
	SVGAutoObjective string        `default:"size" enum:"size,runtime" help:"SVG auto layout objective: size or structural runtime proxies"`
	SVGStyle         string        `default:"legacy" enum:"legacy,auto" help:"SVG paint encoding: compatibility legacy or measured auto selection"`
	SVGPrimitives    string        `default:"snapshots" enum:"snapshots,rect-tracks" help:"SVG primitives: snapshots or experimental retained background rectangles"`
	SVGMaxFPS        int           `default:"0" help:"Maximum SVG timeline FPS, up to 1000; 0 preserves every state"`
}

func (cmd *Cmd) normalizedSVGOptions(format string) (svg.Options, error) {
	options := svg.DefaultOptions()
	if cmd.SVGLayout != "" {
		options.Layout = svg.LayoutMode(strings.ToLower(cmd.SVGLayout))
	}
	if cmd.SVGAnimation != "" {
		options.Animation = svg.AnimationMode(strings.ToLower(cmd.SVGAnimation))
	}
	if cmd.SVGFrameSwitch != "" {
		options.FrameSwitch = svg.FrameSwitchMode(strings.ToLower(cmd.SVGFrameSwitch))
	}
	if cmd.SVGAutoObjective != "" {
		options.AutoObjective = svg.AutoObjective(strings.ToLower(cmd.SVGAutoObjective))
	}
	if cmd.SVGStyle != "" {
		options.Style = svg.StyleMode(strings.ToLower(cmd.SVGStyle))
	}
	if cmd.SVGPrimitives != "" {
		options.Primitives = svg.PrimitiveMode(strings.ToLower(cmd.SVGPrimitives))
	}
	options.MaxFPS = cmd.SVGMaxFPS
	if err := options.Validate(); err != nil {
		return svg.Options{}, err
	}
	if format != "svg" && options != svg.DefaultOptions() {
		return svg.Options{}, fmt.Errorf("svg-specific options cannot be used with %s output", format)
	}
	return options, nil
}

func writeOutput(
	ctx context.Context,
	rdr renderer.Renderer,
	rec *ir.Recording,
	dst io.Writer,
	minifySVG bool,
) error {
	if !minifySVG {
		return rdr.Render(ctx, rec, dst)
	}

	return svgoutput.Write(dst, func(w io.Writer) error { return rdr.Render(ctx, rec, w) })
}

func writeOutputFile(
	ctx context.Context,
	rdr renderer.Renderer,
	rec *ir.Recording,
	dst io.WriteCloser,
	minifySVG bool,
) error {
	return errors.Join(writeOutput(ctx, rdr, rec, dst, minifySVG), dst.Close())
}

func loadCast(path string) (*asciicast.Cast, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return asciicast.Parse(f)
}

// selectTheme picks the theme to render with. The --theme flag wins, then the
// theme embedded in the cast header, then the default theme.
func (cmd *Cmd) selectTheme(cast *asciicast.Cast) theme.Theme {
	selected := theme.Default()
	source := "default"

	if cmd.Theme != "" {
		t, err := theme.Load(cmd.Theme)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load theme %q: %v\n", cmd.Theme, err)
			fmt.Fprintf(os.Stderr, "Falling back to default theme\n")
		} else {
			selected, source = t, "CLI flag"
		}
	} else if cast.Header.Theme.Fg != "" {
		t, err := theme.FromAsciinema("asciicast", cast.Header.Theme.Fg,
			cast.Header.Theme.Bg, cast.Header.Theme.Palette)
		if err != nil {
			if cmd.Debug {
				log.Printf("[Export] Invalid theme in asciicast header: %v", err)
			}
		} else {
			selected, source = t, "asciicast header"
		}
	}

	if cmd.Debug {
		log.Printf("[Export] Using theme from %s: %s", source, selected.Name)
	}

	return selected
}

func (cmd *Cmd) processorConfig(
	selectedTheme *theme.Theme,
	progressCh chan<- progress.Update,
) *ir.ProcessorConfig {
	cfg := ir.DefaultProcessorConfig()
	cfg.Speed = cmd.Speed
	cfg.IdleTimeLimit = cmd.MaxIdle
	cfg.Theme = *selectedTheme
	cfg.ProgressCh = progressCh

	return cfg
}

func (cmd *Cmd) rendererConfig(
	selectedTheme *theme.Theme,
	progressCh chan<- progress.Update,
) *renderer.Config {
	cfg := renderer.DefaultConfig()
	cfg.ShowWindow = !cmd.NoWindow
	cfg.ShowCursor = !cmd.NoCursor
	cfg.Minify = cmd.Minify
	cfg.Debug = cmd.Debug
	cfg.Theme = *selectedTheme
	cfg.ProgressCh = progressCh

	return cfg
}

func newRenderer(format string, cfg *renderer.Config, svgOptions *svg.Options) (renderer.Renderer, error) {
	switch format {
	case "gif":
		r, err := gif.New(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create GIF renderer: %w", err)
		}
		return r, nil
	case "svg":
		return svg.New(
			cfg,
			svg.WithLayout(svgOptions.Layout),
			svg.WithAnimation(svgOptions.Animation),
			svg.WithFrameSwitch(svgOptions.FrameSwitch),
			svg.WithMaxFPS(svgOptions.MaxFPS),
			svg.WithAutoObjective(svgOptions.AutoObjective),
			svg.WithStyleMode(svgOptions.Style),
			svg.WithPrimitiveMode(svgOptions.Primitives),
		), nil
	case "webm":
		r, err := webm.New(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create WebM renderer: %w", err)
		}
		return r, nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

func (cmd *Cmd) Run() error {
	format := strings.ToLower(cmd.Format)
	svgOptions, err := cmd.normalizedSVGOptions(format)
	if err != nil {
		return err
	}
	if svgOptions.Layout == svg.LayoutRegions || svgOptions.Layout == svg.LayoutAuto {
		fmt.Fprintf(os.Stderr, "Warning: --svg-layout=%s can take a while on large recordings\n", svgOptions.Layout)
	}

	output := cmd.Output
	if output == "" {
		output = cmd.File + "." + format
	}

	cast, err := loadCast(cmd.File)
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

	selectedTheme := cmd.selectTheme(cast)

	reporter, progressCh := progress.New()
	reporter.Start()
	exported := false
	defer func() {
		close(progressCh)
		reporter.Wait()
		if exported {
			fmt.Printf("\nExported: %s\n", output)
		}
	}()

	rec, err := ir.NewProcessor(cmd.processorConfig(&selectedTheme, progressCh)).Process(cast)
	if err != nil {
		return err
	}

	rdr, err := newRenderer(format, cmd.rendererConfig(&selectedTheme, progressCh), &svgOptions)
	if err != nil {
		return err
	}

	outFile, err := os.Create(output) //nolint:gosec // output path is from user CLI input
	if err != nil {
		return err
	}

	if err := writeOutputFile(context.Background(), rdr, rec, outFile, cmd.Minify && format == "svg"); err != nil {
		return err
	}

	exported = true
	return nil
}
