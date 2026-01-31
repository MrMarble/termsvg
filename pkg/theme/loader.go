package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load loads a theme by name (built-in) or path (file).
// It checks if the input is a file path (contains "/" or ends with ".json")
// and dispatches to the appropriate loader.
func Load(nameOrPath string) (Theme, error) {
	// Check if it's a file path
	if strings.Contains(nameOrPath, "/") || strings.HasSuffix(nameOrPath, ".json") {
		return LoadFromFile(nameOrPath)
	}
	// Otherwise load built-in theme
	return LoadBuiltin(nameOrPath)
}

// LoadFromFile loads a theme from a JSON file.
// It searches in the following order:
// 1. Direct path (absolute or relative)
// 2. TERMSVG_THEME_PATH environment variable directories
func LoadFromFile(path string) (Theme, error) {
	// Try direct path first
	if _, err := os.Stat(path); err == nil {
		return loadThemeFile(path)
	}

	// Try TERMSVG_THEME_PATH directories
	themePath := os.Getenv("TERMSVG_THEME_PATH")
	if themePath != "" {
		dirs := strings.Split(themePath, string(os.PathListSeparator))
		for _, dir := range dirs {
			fullPath := filepath.Join(dir, path)
			if _, err := os.Stat(fullPath); err == nil {
				return loadThemeFile(fullPath)
			}
			// Also try with .json extension
			if !strings.HasSuffix(path, ".json") {
				fullPath = filepath.Join(dir, path+".json")
				if _, err := os.Stat(fullPath); err == nil {
					return loadThemeFile(fullPath)
				}
			}
		}
	}

	return Theme{}, fmt.Errorf("theme file not found: %s", path)
}

// loadThemeFile reads and parses a theme JSON file.
func loadThemeFile(path string) (Theme, error) {
	data, err := os.ReadFile(path) //nolint:gosec // theme file path is user-provided
	if err != nil {
		return Theme{}, fmt.Errorf("failed to read theme file: %w", err)
	}

	var themeData struct {
		Fg      string `json:"fg"`
		Bg      string `json:"bg"`
		Palette string `json:"palette"`
	}

	if err := json.Unmarshal(data, &themeData); err != nil {
		return Theme{}, fmt.Errorf("failed to parse theme file: %w", err)
	}

	// Use filename as theme name (without extension)
	name := strings.TrimSuffix(filepath.Base(path), ".json")

	return FromAsciinema(name, themeData.Fg, themeData.Bg, themeData.Palette)
}

// LoadBuiltin loads a built-in theme by name.
func LoadBuiltin(name string) (Theme, error) {
	// Normalize name (lowercase, replace spaces with dashes)
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	// Check if theme exists in built-in registry
	if theme, ok := builtinThemes[name]; ok {
		return theme, nil
	}

	return Theme{}, fmt.Errorf("built-in theme not found: %s", name)
}

// IsBuiltin checks if a theme name is a built-in theme.
func IsBuiltin(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	_, ok := builtinThemes[name]
	return ok
}

// ListBuiltin returns a list of available built-in theme names.
func ListBuiltin() []string {
	names := make([]string, 0, len(builtinThemes))
	for name := range builtinThemes {
		names = append(names, name)
	}
	return names
}
