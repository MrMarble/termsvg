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
	file, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	if output == "" {
		output = input
	}

	cast, err := asciicast.Unmarshal(file)
	if err != nil {
		return err
	}

	svg.Export(*cast, output)

	return nil
}
