package color

import (
	"image/color"
	"testing"
)

func TestColorCatalog_Register(t *testing.T) {
	catalog := NewColorCatalog(color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 255})
	palette := Standard()

	// Default colors should return DefaultColorID
	defaultColor := Color{Type: Default}
	id := catalog.Register(defaultColor, palette)
	if id != DefaultColorID {
		t.Errorf("Default color should return DefaultColorID, got %d", id)
	}

	// First non-default color should get ID 1
	red := FromANSI(1)
	id1 := catalog.Register(red, palette)
	if id1 != 1 {
		t.Errorf("First color should get ID 1, got %d", id1)
	}

	// Same color should return same ID (deduplication)
	id1Again := catalog.Register(red, palette)
	if id1Again != id1 {
		t.Errorf("Same color should return same ID, got %d vs %d", id1Again, id1)
	}

	// Different color should get different ID
	blue := FromANSI(4)
	id2 := catalog.Register(blue, palette)
	if id2 == id1 {
		t.Errorf("Different color should get different ID, both got %d", id2)
	}

	// Count should reflect unique colors
	if catalog.Count() != 2 {
		t.Errorf("Count should be 2, got %d", catalog.Count())
	}
}

func TestColorCatalog_Resolved(t *testing.T) {
	catalog := NewColorCatalog(color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 255})
	palette := Standard()

	// Register a color
	red := FromANSI(1)
	id := catalog.Register(red, palette)

	// Resolve should return the RGBA
	resolved := catalog.Resolved(id)
	expected := red.ToRGBA(palette)
	if resolved != expected {
		t.Errorf("Resolved color mismatch: got %v, want %v", resolved, expected)
	}

	// DefaultColorID should return zero RGBA
	defaultResolved := catalog.Resolved(DefaultColorID)
	if defaultResolved != (color.RGBA{}) {
		t.Errorf("Default should resolve to zero RGBA, got %v", defaultResolved)
	}
}

func TestColorCatalog_GenerateClassNames(t *testing.T) {
	catalog := NewColorCatalog(color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 255})
	palette := Standard()

	// Register multiple colors (16 unique ANSI colors)
	for i := uint8(0); i < 16; i++ {
		catalog.Register(FromANSI(i), palette)
	}

	names := catalog.GenerateClassNames()

	// First color should be "a"
	if names[1] != "a" {
		t.Errorf("First class name should be 'a', got %q", names[1])
	}

	// Check sequence
	expectedNames := map[ColorID]string{
		1:  "a",
		2:  "b",
		16: "p",
	}

	for id, expected := range expectedNames {
		if names[id] != expected {
			t.Errorf("ID %d should have name %q, got %q", id, expected, names[id])
		}
	}
}

func TestColorCatalog_DefaultColors(t *testing.T) {
	fg := color.RGBA{200, 200, 200, 255}
	bg := color.RGBA{30, 30, 30, 255}
	catalog := NewColorCatalog(fg, bg)

	if catalog.DefaultForeground() != fg {
		t.Errorf("DefaultForeground mismatch: got %v, want %v", catalog.DefaultForeground(), fg)
	}
	if catalog.DefaultBackground() != bg {
		t.Errorf("DefaultBackground mismatch: got %v, want %v", catalog.DefaultBackground(), bg)
	}
}

func TestColorCatalog_IsDefault(t *testing.T) {
	catalog := NewColorCatalog(color.RGBA{}, color.RGBA{})
	palette := Standard()

	red := FromANSI(1)
	id := catalog.Register(red, palette)

	if catalog.IsDefault(id) {
		t.Error("Non-default color should not be default")
	}
	if !catalog.IsDefault(DefaultColorID) {
		t.Error("DefaultColorID should be default")
	}
}

func TestIDGenerator_Sequence(t *testing.T) {
	gen := newIDGenerator()

	expected := []string{
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j",
		"k", "l", "m", "n", "o", "p", "q", "r", "s", "t",
		"u", "v", "w", "x", "y", "z",
		"aa", "ab", "ac",
	}

	for i, want := range expected {
		got := gen.Next()
		if got != want {
			t.Errorf("Position %d: got %q, want %q", i, got, want)
		}
	}
}

func TestIDGenerator_LongSequence(t *testing.T) {
	gen := newIDGenerator()

	// Generate 702 names (26 + 26*26) to test rollover to "aaa"
	for i := 0; i < 702; i++ {
		gen.Next()
	}

	// Next should be "aaa"
	got := gen.Next()
	if got != "aaa" {
		t.Errorf("After 702 names, expected 'aaa', got %q", got)
	}
}
