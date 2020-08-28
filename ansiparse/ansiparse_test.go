package ansiparse

import (
	"reflect"
	"testing"
)

var MeasureTextAreaTests = []struct {
	text     string
	expected measuredText
}{
	{"test 1", measuredText{1, 6}},
	{"foo", measuredText{1, 3}},
	{"foo\nbar", measuredText{2, 3}},
	{"🇪🇸", measuredText{1, 2}},
	{"こんにちは", measuredText{1, 10}},
}

func TestMeasueTextArea(t *testing.T) {
	for _, tt := range MeasureTextAreaTests {
		t.Run(tt.text, func(t *testing.T) {
			got := measureTextArea(tt.text)
			if got != tt.expected {
				t.Errorf("Expected: %v, got: %v", tt.expected, got)
			}
		})
	}
}

func TestAtomize(t *testing.T) {
	test := "I like to \\u001b[34mmove it\\u001b[39m, move it."
	gotWords, gotAnsies := atomize(test)
	expectedWords := []string{"I like to ", "\\u001b[34m", "move it", "\\u001b[39m", ", move it."}
	expectedAnsies := []string{"\\u001b[34m", "\\u001b[39m"}

	if !reflect.DeepEqual(gotWords, expectedWords) {
		t.Errorf("Expected: %#v, got: %#v", expectedWords, gotWords)
	}
	if !reflect.DeepEqual(gotAnsies, expectedAnsies) {
		t.Errorf("Expected: %#v, got: %#v", expectedAnsies, gotAnsies)
	}
}

func TestParse(t *testing.T) {
	t.Run("gets opening red ansi scape char", func(t *testing.T) {
		text := "\\u001B[31m_"
		expected := "\\u001B[31m"
		got := Parse(text)

		if len(got.chunks) == 0 {
			t.Fatalf("Expected: %#v, got: %#v", expected, got)
		}

		if got.chunks[0].value.ansi != expected {
			t.Errorf("Expected: %v, got: %v", expected, got.chunks[0].value.ansi)
		}
	})
}
