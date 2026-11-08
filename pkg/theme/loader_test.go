package theme

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeTheme(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func validThemeJSON() string {
	return `{"fg":"#ffffff","bg":"#101010","palette":"` + testPalette + `"}`
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.json")
	writeTheme(t, path, validThemeJSON())

	t.Run("builtin name", func(t *testing.T) {
		th, err := Load("dracula")
		if err != nil || th.Name != "dracula" {
			t.Fatalf("Load() = %v, %v", th.Name, err)
		}
	})
	t.Run("file path", func(t *testing.T) {
		th, err := Load(path)
		if err != nil || th.Name != "custom" {
			t.Fatalf("Load() = %v, %v", th.Name, err)
		}
	})
	t.Run("json suffix without slash goes to file loader", func(t *testing.T) {
		if _, err := Load("missing.json"); err == nil || !strings.Contains(err.Error(), "theme file not found") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("unknown builtin", func(t *testing.T) {
		if _, err := Load("nope"); err == nil || !strings.Contains(err.Error(), "built-in theme not found") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, filepath.Join(dir, "envtheme.json"), validThemeJSON())
	writeTheme(t, filepath.Join(dir, "broken.json"), "{not json")
	writeTheme(t, filepath.Join(dir, "badcolor.json"), `{"fg":"x","bg":"#000","palette":""}`)

	t.Run("search path with extension appended", func(t *testing.T) {
		t.Setenv("TERMSVG_THEME_PATH", "/nonexistent"+string(os.PathListSeparator)+dir)
		th, err := LoadFromFile("envtheme")
		if err != nil || th.Name != "envtheme" {
			t.Fatalf("LoadFromFile() = %v, %v", th.Name, err)
		}
	})
	t.Run("search path with extension", func(t *testing.T) {
		t.Setenv("TERMSVG_THEME_PATH", dir)
		if _, err := LoadFromFile("envtheme.json"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		t.Setenv("TERMSVG_THEME_PATH", dir)
		if _, err := LoadFromFile("absent"); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid json", func(t *testing.T) {
		_, err := LoadFromFile(filepath.Join(dir, "broken.json"))
		if err == nil || !strings.Contains(err.Error(), "failed to parse") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("invalid colors", func(t *testing.T) {
		if _, err := LoadFromFile(filepath.Join(dir, "badcolor.json")); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("unreadable path", func(t *testing.T) {
		if _, err := LoadFromFile(dir); err == nil || !strings.Contains(err.Error(), "failed to read") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLoadBuiltin_NormalizesName(t *testing.T) {
	for _, name := range []string{"Gruvbox Dark", "gruvbox-dark", "GRUVBOX-DARK"} {
		th, err := LoadBuiltin(name)
		if err != nil || th.Name != "gruvbox-dark" {
			t.Errorf("LoadBuiltin(%q) = %q, %v", name, th.Name, err)
		}
	}
}

func TestIsBuiltin(t *testing.T) {
	if !IsBuiltin("Solarized Dark") || IsBuiltin("nope") {
		t.Error("IsBuiltin returned unexpected result")
	}
}

func TestListBuiltin(t *testing.T) {
	names := ListBuiltin()
	if len(names) != len(builtinThemes) || !slices.Contains(names, "nord") {
		t.Errorf("names = %v", names)
	}
	for _, n := range names {
		if th, err := LoadBuiltin(n); err != nil || th.Name != n {
			t.Errorf("builtin %q inconsistent: %q, %v", n, th.Name, err)
		}
	}
}
