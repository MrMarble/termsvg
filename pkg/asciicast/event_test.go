package asciicast_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mrmarble/termsvg/pkg/asciicast"
)

func TestJSONMarshal(t *testing.T) {
	tests := map[string]struct {
		input  asciicast.Event
		output string
	}{
		"Input event": {
			input: asciicast.Event{
				Time:      0.05,
				EventType: asciicast.Input,
				EventData: "input",
			},
			output: `[0.05,"i","input"]`,
		},
		"Output event": {
			input: asciicast.Event{
				Time:      0.25,
				EventType: asciicast.Output, EventData: "output",
			},
			output: `[0.25,"o","output"]`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			input := tc.input

			output, err := json.Marshal(&input)
			if err != nil {
				t.Fatal(err)
			}

			diff := cmp.Diff(string(output), tc.output)
			if diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestJSONUnmarshal(t *testing.T) {
	tests := map[string]struct {
		input  string
		output asciicast.Event
	}{
		"Input event": {
			output: asciicast.Event{
				Time:      0.05,
				EventType: asciicast.Input,
				EventData: "input",
			},
			input: `[0.05,"i","input"]`,
		},
		"Output event": {
			output: asciicast.Event{
				Time:      0.25,
				EventType: asciicast.Output, EventData: "output",
			},
			input: `[0.25,"o","output"]`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var output asciicast.Event
			err := json.Unmarshal([]byte(tc.input), &output)
			if err != nil {
				t.Fatal(err)
			}
			diff := cmp.Diff(output, tc.output)
			if diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
