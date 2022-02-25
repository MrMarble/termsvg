package export

import (
	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/svg"
)

type Cmd struct {
	Input  string  `short:"i" type:"existingfile" help:"asciicast file to export"`
	Output float64 `optional:"" short:"o" help:"where to save the file"`
}

func (cmd *Cmd) Run() error {
	records, err := asciicast.ReadRecords(cmd.Input)
	if err != nil {
		return err
	}

	svg.New(*records)

	return nil
}
