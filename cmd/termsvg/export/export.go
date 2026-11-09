//nolint:lll // Kong option tags cannot be split across lines.
package export

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/mrmarble/termsvg/cmd/termsvg/internal/pacing"
	"github.com/mrmarble/termsvg/internal/asciicast"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/progress"
	"github.com/mrmarble/termsvg/internal/renderer"
	"github.com/mrmarble/termsvg/internal/renderer/gif"
	"github.com/mrmarble/termsvg/internal/renderer/svg"
	"github.com/mrmarble/termsvg/internal/renderer/webm"
	"github.com/mrmarble/termsvg/internal/theme"
)

type Cmd struct {
	File             string `arg:"" type:"existingfile" help:"Asciicast file to export"`
	Output           string `short:"o" type:"path" help:"Output file path (default: <input>.<format>)"`
	Format           string `short:"f" default:"svg" enum:"svg,gif,webm" help:"Output format (svg, gif, webm)"`
	Minify           bool   `short:"m" help:"Minify output (SVG only)"`
	NoWindow         bool   `short:"n" help:"Don't render terminal window chrome"`
	NoCursor         bool   `short:"C" help:"Don't render cursor"`
	pacing.Flags     `embed:""`
	Cols             int    `short:"c" default:"0" help:"Override columns (0 = use original)"`
	Rows             int    `short:"r" default:"0" help:"Override rows (0 = use original)"`
	Debug            bool   `short:"d" help:"Enable debug logging"`
	SystemFonts      bool   `help:"Draw glyphs the embedded fonts lack with installed fonts (GIF and WebM)"`
	Theme            string `short:"t" help:"Theme name (built-in) or path to theme JSON file"`
	SVGLayout        string `default:"frames" enum:"frames,bands,regions,scroll,auto" help:"SVG layout: frames, experimental bands/regions/scroll, or measured auto selection"`
	SVGAnimation     string `default:"css" enum:"css,smil" help:"SVG animation backend (css or experimental smil)"`
	SVGFrameSwitch   string `default:"translate" enum:"translate,href" help:"SVG state switching: translated strips or experimental animated hrefs"`
	SVGAutoObjective string `default:"size" enum:"size,runtime" help:"SVG auto layout objective: size or structural runtime proxies"`
	SVGStyle         string `default:"legacy" enum:"legacy,auto" help:"SVG paint encoding: compatibility legacy or measured auto selection"`
	SVGPrimitives    string `default:"snapshots" enum:"snapshots,rect-tracks" help:"SVG primitives: snapshots or experimental retained background rectangles"`
	SVGMaxFPS        int    `default:"0" help:"Maximum SVG timeline FPS, up to 1000; 0 preserves every state"`
}

func (cmd *Cmd) svgOptions() svg.Options {
	return svg.Options{
		Layout:        svg.LayoutMode(cmd.SVGLayout),
		Animation:     svg.AnimationMode(cmd.SVGAnimation),
		FrameSwitch:   svg.FrameSwitchMode(cmd.SVGFrameSwitch),
		AutoObjective: svg.AutoObjective(cmd.SVGAutoObjective),
		Style:         svg.StyleMode(cmd.SVGStyle),
		Primitives:    svg.PrimitiveMode(cmd.SVGPrimitives),
		MaxFPS:        cmd.SVGMaxFPS,
	}.Normalize()
}

// checkFormatFlags rejects flags that do not apply to format and returns the
// SVG options in effect.
func (cmd *Cmd) checkFormatFlags(format string) (svg.Options, error) {
	options := cmd.svgOptions()
	if format == "svg" {
		if cmd.SystemFonts {
			return svg.Options{}, errors.New("--system-fonts only applies to gif and webm output: an SVG uses the viewer's fonts")
		}
		return options, options.Validate()
	}
	if err := options.Validate(); err != nil {
		return svg.Options{}, err
	}
	if options != svg.DefaultOptions() {
		return svg.Options{}, fmt.Errorf("svg-specific options cannot be used with %s output", format)
	}
	return options, nil
}

func writeOutputFile(ctx context.Context, rdr renderer.Renderer, rec *ir.Recording, dst io.WriteCloser) error {
	return errors.Join(rdr.Render(ctx, rec, dst), dst.Close())
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
	idleLimit time.Duration,
	selectedTheme theme.Theme,
	progressCh chan<- progress.Update,
) *ir.ProcessorConfig {
	cfg := ir.DefaultProcessorConfig()
	cfg.Speed = cmd.Speed
	cfg.IdleTimeLimit = idleLimit
	cfg.Theme = selectedTheme
	cfg.ProgressCh = progressCh

	return cfg
}

func (cmd *Cmd) rendererConfig(
	selectedTheme theme.Theme,
	progressCh chan<- progress.Update,
) *renderer.Config {
	cfg := renderer.DefaultConfig()
	cfg.ShowWindow = !cmd.NoWindow
	cfg.ShowCursor = !cmd.NoCursor
	cfg.Minify = cmd.Minify
	cfg.Debug = cmd.Debug
	cfg.SystemFonts = cmd.SystemFonts
	cfg.Theme = selectedTheme
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
		return svg.New(cfg, svg.WithOptions(*svgOptions)), nil
	default: // webm; Kong's enum rejects any other format
		r, err := webm.New(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create WebM renderer: %w", err)
		}
		return r, nil
	}
}

func (cmd *Cmd) Run() error {
	format := cmd.Format
	svgOptions, err := cmd.checkFormatFlags(format)
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

	cast, err := asciicast.ParseFile(cmd.File)
	if err != nil {
		return err
	}

	if cmd.Cols > 0 {
		cast.Header.Width = cmd.Cols
	}
	if cmd.Rows > 0 {
		cast.Header.Height = cmd.Rows
	}

	selectedTheme := cmd.selectTheme(cast)

	err = progress.Run(func(progressCh chan<- progress.Update) error {
		rec, err := ir.NewProcessor(cmd.processorConfig(cast.IdleLimit(cmd.MaxIdle), selectedTheme, progressCh)).Process(cast)
		if err != nil {
			return err
		}

		rdr, err := newRenderer(format, cmd.rendererConfig(selectedTheme, progressCh), &svgOptions)
		if err != nil {
			return err
		}

		_, statErr := os.Stat(output)
		isNewFile := errors.Is(statErr, os.ErrNotExist)

		outFile, err := os.Create(output) //nolint:gosec // output path is from user CLI input
		if err != nil {
			return err
		}

		if err := writeOutputFile(context.Background(), rdr, rec, outFile); err != nil {
			if isNewFile {
				_ = os.Remove(output)
			}
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nExported: %s\n", output)
	return nil
}
