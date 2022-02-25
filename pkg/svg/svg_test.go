package svg_test

import (
	"testing"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/svg"
)

func TestNew(t *testing.T) {
	records, err := asciicast.ReadRecords("/home/mrmarble/repos/termsvg/svg.cast")
	if err != nil {
		t.Fatal(err)
	}

	svg.New(*records)
}
