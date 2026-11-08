package main

import (
	"fmt"
	"io"

	"github.com/alecthomas/kong"
	"github.com/mrmarble/termsvg/cmd/termsvg/export"
	"github.com/mrmarble/termsvg/cmd/termsvg/play"
)

// VersionFlag prints version information and exits when set.
type VersionFlag string

// CLI is the termsvg command tree. Commands that are not available on every
// platform live in platformCommands.
type CLI struct {
	Version VersionFlag `name:"version" help:"Print version information and quit"`

	Play   play.Cmd   `cmd:"" help:"Play back a recorded terminal session"`
	Export export.Cmd `cmd:"" help:"Export asciicast to SVG"`

	platformCommands `embed:""`
}

// Version info (populated by goreleaser).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func (v VersionFlag) Decode(_ *kong.DecodeContext) error { return nil }
func (v VersionFlag) IsBool() bool                       { return true }
func (v VersionFlag) BeforeApply(app *kong.Kong) error {
	fmt.Fprintf(app.Stdout, "termsvg %s (%s) built on %s\n", version, commit, date)
	app.Exit(0)
	return nil
}

// run parses args and executes the selected command. exit is called when the
// parser terminates the process itself (--help, --version, usage errors).
func run(args []string, stdout, stderr io.Writer, exit func(int)) error {
	var cli CLI

	parser, err := kong.New(&cli,
		kong.Name("termsvg"),
		kong.Description("Record, play, and export terminal sessions as SVG animations"),
		kong.UsageOnError(),
		kong.Writers(stdout, stderr),
		kong.Exit(exit),
	)
	if err != nil {
		return err
	}

	ctx, err := parser.Parse(args)
	if err != nil {
		return err
	}

	return ctx.Run()
}
