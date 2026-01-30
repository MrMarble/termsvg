//go:build windows

package main

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/mrmarble/termsvg/cmd/termsvg/export"
	"github.com/mrmarble/termsvg/cmd/termsvg/play"
)

// Version info (populated by goreleaser)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type VersionFlag string

func (v VersionFlag) Decode(_ *kong.DecodeContext) error { return nil }
func (v VersionFlag) IsBool() bool                       { return true }
func (v VersionFlag) BeforeApply(app *kong.Kong) error {
	fmt.Printf("termsvg %s (%s) built on %s\n", version, commit, date)
	app.Exit(0)
	return nil
}

func main() {
	var cli struct {
		Version VersionFlag `name:"version" help:"Print version information and quit"`

		Play   play.Cmd   `cmd:"" help:"Play back a recorded terminal session"`
		Export export.Cmd `cmd:"" help:"Export asciicast to SVG"`
	}

	ctx := kong.Parse(&cli,
		kong.Name("termsvg"),
		kong.Description("Record, play, and export terminal sessions as SVG animations"),
		kong.UsageOnError(),
	)

	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
