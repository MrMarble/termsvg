//go:build windows

package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/mrmarble/termsvg/cmd/termsvg/export"
	"github.com/mrmarble/termsvg/cmd/termsvg/play"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Context struct {
	Debug bool
}

type VersionFlag string

var (
	// Populated by goreleaser during build
	version = "master"
	commit  = "?"
	date    = ""
)

func init() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, PartsExclude: []string{"time"}})
}

func (v VersionFlag) Decode(_ *kong.DecodeContext) error { return nil }
func (v VersionFlag) IsBool() bool                       { return true }
func (v VersionFlag) BeforeApply(app *kong.Kong) error {
	fmt.Printf("termsvg has version %s built from %s on %s\n", version, commit, date)
	app.Exit(0)

	return nil
}

func main() {
	var cli struct {
		Debug   bool        `help:"Enable debug mode."`
		Version VersionFlag `name:"version" help:"Print version information and quit"`

		Play   play.Cmd   `cmd:"" help:"Play a recording."`
		Export export.Cmd `cmd:"" help:"Export asciicast."`
	}

	ctx := kong.Parse(&cli,
		kong.Name("termsvg"),
		kong.Description("A cli tool for recording terminal sessions"),
		kong.UsageOnError())
	// Call the Run() method of the selected parsed command.
	err := ctx.Run(&Context{Debug: cli.Debug})
	ctx.FatalIfErrorf(err)
}
