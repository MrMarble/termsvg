package uniqueid_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mrmarble/termsvg/internal/uniqueid"
)

func TestUniqueID(t *testing.T) {
	tests := map[string]struct {
		input  string
		output string
	}{
		"Initial":    {uniqueid.New().String(), "a"},
		"Next":       {runTimes(t, 1).String(), "b"},
		"10 Times":   {runTimes(t, 10).String(), "k"},
		"25 Times":   {runTimes(t, 25).String(), "z"},
		"26 Times":   {runTimes(t, 26).String(), "aa"},
		"27 Times":   {runTimes(t, 27).String(), "ab"},
		"51 Times":   {runTimes(t, 51).String(), "az"},
		"52 Times":   {runTimes(t, 52).String(), "ba"},
		"53 Times":   {runTimes(t, 53).String(), "bb"},
		"150 Times":  {runTimes(t, 150).String(), "eu"},
		"1500 Times": {runTimes(t, 1500).String(), "zzes"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			diff(t, test.input, test.output)
		})
	}
}

func runTimes(t *testing.T, times int) *uniqueid.ID {
	t.Helper()

	id := uniqueid.New()
	for i := 0; i < times; i++ {
		id.Next()
	}

	return id
}

func diff(t *testing.T, x interface{}, y interface{}) {
	t.Helper()

	diff := cmp.Diff(x, y)
	if diff != "" {
		t.Fatal(diff)
	}
}
