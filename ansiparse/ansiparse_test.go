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
	got := atomize(test)
	expected := struct {
		words  []string
		ansies []string
	}{
		[]string{"I like to ", "\\u001b[34m", "move it", "\\u001b[39m", ", move it."},
		[]string{"\\u001b[34m", "\\u001b[39m"},
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected: %#v, got: %#v", expected, got)
	}
}
