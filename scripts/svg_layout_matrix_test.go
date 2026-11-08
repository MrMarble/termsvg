package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSVGLayoutMatrixCapturesNineVariantsAtTwoFrameRates(t *testing.T) {
	dir := t.TempDir()
	cast := filepath.Join(dir, "fixture.cast")
	writeTestFile(t, cast, "fixture")
	fake := filepath.Join(dir, "termsvg")
	writeTestFile(t, fake, `#!/usr/bin/env bash
set -eu
out=
for ((i=1;i<=$#;i++)); do
  if [[ ${!i} == -o ]]; then j=$((i+1)); out=${!j}; fi
done
printf '<svg><g transform="translate(20)"><text>x</text></g></svg>' > "$out"
`)
	if err := os.Chmod(fake, 0o700); err != nil { //nolint:gosec // executable test fixture
		t.Fatal(err)
	}
	cmd := exec.Command("./scripts/svg-layout-matrix.sh", cast) //nolint:gosec // repository script and temp fixture
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "TERMSVG="+fake)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("matrix: %v\n%s", err, output)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 20 {
		t.Fatalf("matrix lines = %d, want comment + header + 18 rows\n%s", len(lines), output)
	}
	if strings.Contains(string(output), "auto-") {
		t.Fatalf("matrix contains unsupported auto candidate:\n%s", output)
	}
	for _, want := range []string{"lossless-frames-css-translate", "30fps-bands-smil-href", "lossless-regions-smil-href"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("matrix missing %s:\n%s", want, output)
		}
	}
}
