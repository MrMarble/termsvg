package stripansi

import (
	"reflect"
	"testing"
)

func TestAnsiRegex(t *testing.T) {
	test := "\u001B[4mcake\u001B[0m"
	got := AnsiRegex.FindAllString(test, -1)
	expected := []string{"\u001B[4m", "\u001B[0m"}
	test2 := "cake"

	if AnsiRegex.MatchString(test2) {
		t.Errorf("Expected: %#v, got: %#v", false, AnsiRegex.MatchString(test2))
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected: %#v, got: %#v", expected, got)
	}
}

func TestStript(t *testing.T) {
	text := "\u001B[4mcake\u001B[0m"
	got := Strip(text)
	expected := "cake"

	if got != expected {
		t.Errorf("Expected: %#v, got: %#v", expected, got)
	}
}
