package raster

import "github.com/mrmarble/termsvg/internal/theme"

func testConfig() Config {
	return Config{Theme: theme.Default(), ShowWindow: true, ShowCursor: true, FontSize: 20}
}
