package testutils

import (
	"io"
	"os"

	"github.com/google/go-cmp/cmp"
)

type Helper interface {
	Helper()
	Fatalf(string, ...interface{})
}

func GoldenData(t Helper, identifier string) []byte {
	t.Helper()

	goldenPath := "testdata/" + identifier + ".golden"

	//nolint:gosec
	f, err := os.Open(goldenPath)
	if err != nil {
		t.Fatalf("Error opening file %s: %s", goldenPath, err)
	}

	//nolint:errcheck
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Error reading file %s: %s", goldenPath, err)
	}

	return data
}

func Diff(t Helper, x interface{}, y interface{}) {
	t.Helper()

	diff := cmp.Diff(x, y)
	if diff != "" {
		t.Fatalf(diff)
	}
}
