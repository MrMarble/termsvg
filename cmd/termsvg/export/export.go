package export

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/mrmarble/termsvg/internal/svg"
	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/rs/zerolog/log"
	"github.com/tdewolff/minify/v2"
	msvg "github.com/tdewolff/minify/v2/svg"
)

type Cmd struct {
	File            string `arg:"" type:"existingfile" help:"asciicast file to export"`
	Output          string `optional:"" short:"o" type:"path" help:"where to save the file. Defaults to <input_file>.svg"`
	Mini            bool   `name:"minify" optional:"" short:"m" help:"minify output file. May be slower"`
	NoWindow        bool   `name:"nowindow" optional:"" short:"n" help:"don't render terminal window in svg"`
	BackgroundColor string `optional:"" short:"b" help:"background color in hexadecimal format (e.g. #FFFFFF)"`
	TextColor       string `optional:"" short:"t" help:"text color in hexadecimal format (e.g. #000000)"`
}

func (cmd *Cmd) Run() error {
	output := cmd.Output
	if output == "" {
		output = cmd.File + ".svg"
	}

	err := export(cmd.File, output, cmd.Mini, cmd.BackgroundColor, cmd.TextColor, cmd.NoWindow)
	if err != nil {
		return err
	}

	log.Info().Str("output", output).Msg("svg file saved.")

	return nil
}

func export(input, output string, mini bool, bgColor, textColor string, noWindow bool) error {
	inputFile, err := os.ReadFile(filepath.Clean(input))
	if err != nil {
		return err
	}

	cast, err := asciicast.Unmarshal(inputFile)
	if err != nil {
		return err
	}

	out := new(bytes.Buffer)
	var data []byte

	svg.Export(*cast, out, bgColor, textColor, noWindow)
	if mini {
		m := minify.New()
		m.AddFunc("image/svg+xml", msvg.Minify)
		b, err := m.Bytes("image/svg+xml", out.Bytes())
		if err != nil {
			return err
		}
		data = b
	} else {
		data = out.Bytes()
	}
	outputFile, err := os.Create(output)
	if err != nil {
		return err
	}
	_, err = outputFile.Write(data)
	if err != nil {
		//nolint:gosec,errcheck
		outputFile.Close()
		return err
	}

	return outputFile.Close()
}
