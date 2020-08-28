package ansiparse

import (
	"reflect"
	"testing"
)

func TestSplitString(t *testing.T) {
	test := "foo bar"
	delimiter := " "
	got := splitString(test, delimiter)
	expected := []string{"foo", " ", "bar"}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected: %#v, got: %#v", expected, got)
	}
}

func TestSplitSlice(t *testing.T) {
	test := []string{"foo bar", "bar foo"}
	delimiter := " "
	got := splitSlice(test, delimiter)
	expected := []string{"foo", " ", "bar", "bar", " ", "foo"}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected: %#v, got: %#v", expected, got)
	}
}

var SuperSplitTests = []struct {
	text       interface{}
	delimiters []string
	expected   []string
}{
	{"I like to move it move it.", []string{"to", "it"}, []string{"I like ", "to", " move ", "it", " move ", "it", "."}},
	{"A+B-C", []string{"+", "-"}, []string{"A", "+", "B", "-", "C"}},
	{"I like to \u001b[34mmove it\u001b[39m, move it.", []string{"\u001b[34m", "\u001b[39m"}, []string{"I like to ", "\u001b[34m", "move it", "\u001b[39m", ", move it."}},
}

func TestSuperSPlit(t *testing.T) {
	for _, tt := range SuperSplitTests {
		t.Run(tt.text.(string), func(t *testing.T) {
			got := superSplit(tt.text, tt.delimiters)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Expected: %v, got: %v", tt.expected, got)
			}
		})
	}
}
