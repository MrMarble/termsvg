package stripansi

import (
	"testing"
)

func TestStript(t *testing.T) {
	text := "\u001B[4mcake\u001B[0m"
	expected := "cake"

	if Strip(text) == expected {
		t.Fatal("Regex is not working")
	}
}
