package export

import (
	"os"

	"github.com/mrmarble/termsvg/internal/svg"
	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/rs/zerolog/log"
)

type Cmd struct {
	Input  string `short:"i" type:"existingfile" help:"asciicast file to export"`
	Output string `optional:"" short:"o" type:"path" help:"where to save the file"`
}

func (cmd *Cmd) Run() error {
	output := cmd.Output
	if output == "" {
		output = cmd.Input + ".svg"
	}

	err := export(cmd.Input, output)
	if err != nil {
		return err
	}

	log.Info().Str("output", output).Msg("svg file saved.")

	return nil
}

func export(input, output string) error {
	inputFile, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	cast, err := asciicast.Unmarshal(inputFile)
	if err != nil {
		return err
	}

	outputFile, err := os.Create(output)
	if err != nil {
		return err
	}

	defer outputFile.Close()

	svg.Export(*cast, outputFile)

	return nil
}
