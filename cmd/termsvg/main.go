package main

import (
	"os"

	"github.com/alecthomas/kong"
	"github.com/mrmarble/termsvg/cmd/termsvg/export"
	"github.com/mrmarble/termsvg/cmd/termsvg/play"
	"github.com/mrmarble/termsvg/cmd/termsvg/rec"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Context struct {
	Debug bool
}

func init() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, PartsExclude: []string{"time"}})
}

func main() {
	var cli struct {
		Debug bool `help:"Enable debug mode."`

		Play   play.Cmd   `cmd:"" help:"Play a recording."`
		Rec    rec.Cmd    `cmd:"" help:"Record a terminal sesion."`
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
