package color

import (
	"image/color"
)

// ColorID is a unique identifier for a color in the catalog.
// Value 0 represents "default" (no explicit color set).
type ColorID uint16

// DefaultColorID represents the default/unset color.
const DefaultColorID ColorID = 0

// ColorCatalog maps unique colors to stable IDs for efficient referencing.
// It deduplicates colors and provides CSS class name generation.
type ColorCatalog struct {
	// colors maps ColorID to the resolved RGBA value
	colors map[ColorID]color.RGBA

	// lookup maps color key to ColorID for deduplication
	lookup map[colorKey]ColorID

	// nextID is the next available ID
	nextID ColorID

	// defaultFG and defaultBG are the theme defaults
	defaultFG color.RGBA
	defaultBG color.RGBA
}

// colorKey is used for deduplication - represents the unique identity of a color.
type colorKey struct {
	r, g, b uint8
}

// NewColorCatalog creates a color catalog with the given default colors.
func NewColorCatalog(defaultFG, defaultBG color.RGBA) *ColorCatalog {
	return &ColorCatalog{
		colors:    make(map[ColorID]color.RGBA),
		lookup:    make(map[colorKey]ColorID),
		nextID:    1, // 0 is reserved for DefaultColorID
		defaultFG: defaultFG,
		defaultBG: defaultBG,
	}
}

// Register adds a color to the catalog and returns its ID.
// If the color already exists, returns the existing ID.
// Default colors return DefaultColorID.
func (c *ColorCatalog) Register(col Color, palette Palette) ColorID {
	// Default colors get the special ID
	if col.Type == Default {
		return DefaultColorID
	}

	// Resolve to RGBA
	rgba := col.ToRGBA(palette)
	key := colorKey{r: rgba.R, g: rgba.G, b: rgba.B}

	// Check if already registered
	if id, exists := c.lookup[key]; exists {
		return id
	}

	// Register new color
	id := c.nextID
	c.nextID++
	c.colors[id] = rgba
	c.lookup[key] = id

	return id
}

// Resolved returns the RGBA value for a ColorID.
// For DefaultColorID, returns a zero RGBA (caller should use theme default).
func (c *ColorCatalog) Resolved(id ColorID) color.RGBA {
	if id == DefaultColorID {
		return color.RGBA{}
	}
	return c.colors[id]
}

// IsDefault checks if the ColorID represents a default color.
func (c *ColorCatalog) IsDefault(id ColorID) bool {
	return id == DefaultColorID
}

// All returns all color entries for iteration (e.g., generating CSS classes).
func (c *ColorCatalog) All() map[ColorID]color.RGBA {
	return c.colors
}

// Count returns the number of unique colors (excluding default).
func (c *ColorCatalog) Count() int {
	return len(c.colors)
}

// DefaultForeground returns the default foreground color.
func (c *ColorCatalog) DefaultForeground() color.RGBA {
	return c.defaultFG
}

// DefaultBackground returns the default background color.
func (c *ColorCatalog) DefaultBackground() color.RGBA {
	return c.defaultBG
}

// GenerateClassNames creates CSS class names for all colors.
// Returns a map from ColorID to class name (a, b, ..., z, aa, ab...).
func (c *ColorCatalog) GenerateClassNames() map[ColorID]string {
	names := make(map[ColorID]string)
	gen := newIDGenerator()

	// Generate names in ID order for deterministic output
	for id := ColorID(1); id < c.nextID; id++ {
		names[id] = gen.Next()
	}

	return names
}

// idGenerator produces CSS class names: a, b, ..., z, aa, ab, ...
type idGenerator struct {
	current []byte
}

func newIDGenerator() *idGenerator {
	return &idGenerator{current: []byte{'a' - 1}}
}

func (gen *idGenerator) Next() string {
	for i := len(gen.current) - 1; i >= 0; i-- {
		if gen.current[i] < 'z' {
			gen.current[i]++
			return string(gen.current)
		}
		gen.current[i] = 'a'
	}
	gen.current = append([]byte{'a'}, gen.current...)
	return string(gen.current)
}
