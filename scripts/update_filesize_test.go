package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateFilesizeUsesConventionalReduction(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "git", "init", "-q")
	mustRun(t, dir, "git", "config", "user.name", "Test")
	mustRun(t, dir, "git", "config", "user.email", "test@example.com")
	if err := os.Mkdir(filepath.Join(dir, "examples"), 0o750); err != nil {
		t.Fatal(err)
	}
	readme := "before\n<!--SIZES_START-->\nold\n<!--SIZES_END-->\nafter\n"
	writeTestFile(t, filepath.Join(dir, "examples", "README.md"), readme)
	writeTestFile(t, filepath.Join(dir, "examples", "sample.svg"), strings.Repeat("x", 100))
	mustRun(t, dir, "git", "add", "examples")
	mustRun(t, dir, "git", "commit", "-qm", "fixture")
	writeTestFile(t, filepath.Join(dir, "examples", "sample.svg"), strings.Repeat("x", 75))

	script, err := os.ReadFile("update-filesize.sh")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "update-filesize.sh")
	writeTestFile(t, path, string(script))
	if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // executable test fixture
		t.Fatal(err)
	}
	mustRun(t, dir, path)

	//nolint:gosec // path is rooted in t.TempDir.
	got, err := os.ReadFile(filepath.Join(dir, "examples", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "| sample.svg | 1 | 100.00B | 75.00B | 25.0000% |") {
		t.Fatalf("README table = %q", got)
	}
}

func writeTestFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil { //nolint:gosec // test-controlled path
		t.Fatal(err)
	}
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...) //nolint:gosec // test invokes fixed tools and temp scripts
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
}
