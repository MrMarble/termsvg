package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPalette = "#000000:#111111:#222222:#333333:#444444:#555555:#666666:#777777:#888888:#999999:#aaaaaa:#bbbbbb:#cccccc:#dddddd:#eeeeee:#ffffff"

func TestColorToGo(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "long with hash", in: "#ff5733", want: "{R: 255, G: 87, B: 51, A: 255}"},
		{name: "long without hash", in: "0a141e", want: "{R: 10, G: 20, B: 30, A: 255}"},
		{name: "short form", in: "#f80", want: "{R: 255, G: 136, B: 0, A: 255}"},
		{name: "wrong length falls back", in: "#ffff", want: defaultRGBA},
		{name: "empty falls back", in: "", want: defaultRGBA},
		{name: "invalid red", in: "#zz0000", want: defaultRGBA},
		{name: "invalid green", in: "#00zz00", want: defaultRGBA},
		{name: "invalid blue", in: "#0000zz", want: defaultRGBA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := colorToGo(tt.in); got != tt.want {
				t.Errorf("colorToGo(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHexToRGBA(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "long", in: "#ff5733", want: "R: 255, G: 87, B: 51, A: 255"},
		{name: "short", in: "abc", want: "R: 170, G: 187, B: 204, A: 255"},
		{name: "wrong length falls back", in: "12", want: defaultRGBAPlain},
		{name: "invalid red", in: "#zz0000", want: defaultRGBAPlain},
		{name: "invalid green", in: "#00zz00", want: defaultRGBAPlain},
		{name: "invalid blue", in: "#0000zz", want: defaultRGBAPlain},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hexToRGBA(tt.in); got != tt.want {
				t.Errorf("hexToRGBA(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestToVarName(t *testing.T) {
	tests := map[string]string{
		"dracula":        "Dracula",
		"gruvbox-dark":   "GruvboxDark",
		"solarized-dark": "SolarizedDark",
		"a-b-c":          "ABC",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			if got := toVarName(in); got != want {
				t.Errorf("toVarName(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestParseThemeFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("valid", func(t *testing.T) {
		p := write("my-theme.json", `{"fg":"#ffffff","bg":"#000000","palette":"`+validPalette+`"}`)
		info, err := parseThemeFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Name != "my-theme" || info.VarName != "MyTheme" {
			t.Errorf("name/var = %q/%q", info.Name, info.VarName)
		}
		if info.Fg != "R: 255, G: 255, B: 255, A: 255" || info.WindowBg != info.Bg {
			t.Errorf("colors = %+v", info)
		}
		if n := strings.Count(info.PaletteOverrides, "p["); n != 16 {
			t.Errorf("palette overrides = %d, want 16", n)
		}
	})
	t.Run("wrong palette size", func(t *testing.T) {
		p := write("short.json", `{"fg":"#fff","bg":"#000","palette":"#000:#fff"}`)
		if _, err := parseThemeFile(p); err == nil || !strings.Contains(err.Error(), "16 colors") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("invalid json", func(t *testing.T) {
		if _, err := parseThemeFile(write("bad.json", "{")); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("missing file", func(t *testing.T) {
		if _, err := parseThemeFile(filepath.Join(dir, "absent.json")); err == nil {
			t.Error("expected error")
		}
	})
}
