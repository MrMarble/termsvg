package svg_test

import (
	"bytes"
	"testing"

	"github.com/mrmarble/termsvg/internal/svg"
	"github.com/mrmarble/termsvg/internal/testutils"
	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/sebdah/goldie/v2"
)

func TestExport(t *testing.T) {
	input := testutils.GoldenData(t, "TestExportInput")

	cast, err := asciicast.Unmarshal(input)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	// Pass empty override bg and text colors
	svg.Export(*cast, &output, "", "", false)

	g := goldie.New(t)
	g.Assert(t, "TestExportOutput", output.Bytes())
}

func TestNoWindow(t *testing.T) {
	input := testutils.GoldenData(t, "TestExportInput")

	cast, err := asciicast.Unmarshal(input)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	// Pass empty override bg and text colors
	svg.Export(*cast, &output, "", "", true)

	g := goldie.New(t)
	g.Assert(t, "TestExportOutputNoWindow", output.Bytes())
}

func BenchmarkExport(b *testing.B) {
	input := testutils.GoldenData(b, "TestExportInput")

	cast, err := asciicast.Unmarshal(input)
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		var output bytes.Buffer

		// Pass empty override bg and text colors
		svg.Export(*cast, &output, "", "", false)
	}
}
