package export

import (
	"os"

	"github.com/fatih/color"
	"github.com/mrmarble/termsvg/internal/svg"
	"github.com/mrmarble/termsvg/pkg/asciicast"
)

type Cmd struct {
	Input  string `short:"i" type:"existingfile" help:"asciicast file to export"`
	Output string `optional:"" short:"o" help:"where to save the file"`
}

func (cmd *Cmd) Run() error {
	err := export(cmd.Input, cmd.Output)
	if err != nil {
		return err
	}

	color.Green("Svg file saved to %s", cmd.Output)

	return nil
}

func export(input, output string) error {
	inputFile, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	if output == "" {
		output = input + ".svg"
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
