package color

import (
	"image/color"
)

// ID is a unique identifier for a color in the catalog.
// Value 0 represents "default" (no explicit color set).
type ID uint16

// Catalog maps unique colors to stable IDs for efficient referencing.
// It deduplicates colors and provides CSS class name generation.
type Catalog struct {
	// colors holds the resolved RGBA value of ID i+1, in order of first use
	colors []color.RGBA

	// lookup maps color key to ID for deduplication
	lookup map[colorKey]ID

	// defaultFG and defaultBG are the theme defaults
	defaultFG color.RGBA
	defaultBG color.RGBA
}

// colorKey is used for deduplication - represents the unique identity of a color.
type colorKey struct {
	r, g, b uint8
}

// idGenerator produces CSS class names: a, b, ..., z, aa, ab, ...
type idGenerator struct {
	current []byte
}

// DefaultID represents the default/unset color.
const DefaultID ID = 0

// NewCatalog creates a color catalog with the given default colors.
func NewCatalog(defaultFG, defaultBG color.RGBA) *Catalog {
	return &Catalog{
		lookup:    make(map[colorKey]ID),
		defaultFG: defaultFG,
		defaultBG: defaultBG,
	}
}

// Register adds a color to the catalog and returns its ID.
// If the color already exists, returns the existing ID.
// Default colors return DefaultID.
func (c *Catalog) Register(col Color, palette *Palette) ID {
	if col.Type == Default {
		return DefaultID
	}

	rgba := col.ToRGBA(palette)
	key := colorKey{r: rgba.R, g: rgba.G, b: rgba.B}

	if id, exists := c.lookup[key]; exists {
		return id
	}

	c.colors = append(c.colors, rgba)
	id := ID(len(c.colors)) //nolint:gosec // a recording does not use anywhere near 65535 distinct colors
	c.lookup[key] = id

	return id
}

// Resolved returns the RGBA value for an ID.
// For DefaultID, returns a zero RGBA (caller should use theme default).
func (c *Catalog) Resolved(id ID) color.RGBA {
	if id == DefaultID || int(id) > len(c.colors) {
		return color.RGBA{}
	}
	return c.colors[id-1]
}

// IsDefault checks if the ID represents a default color.
func (c *Catalog) IsDefault(id ID) bool {
	return id == DefaultID
}

// Colors returns the registered colors in ID order, which is the order they
// were first registered in. The slice is the catalog's own and must not be modified.
func (c *Catalog) Colors() []color.RGBA {
	return c.colors
}

// DefaultForeground returns the default foreground color.
func (c *Catalog) DefaultForeground() color.RGBA {
	return c.defaultFG
}

// DefaultBackground returns the default background color.
func (c *Catalog) DefaultBackground() color.RGBA {
	return c.defaultBG
}

// GenerateClassNames creates CSS class names for all colors.
// Returns a map from ID to class name (a, b, ..., z, aa, ab...).
func (c *Catalog) GenerateClassNames() map[ID]string {
	names := make(map[ID]string)
	gen := newIDGenerator()

	for id := ID(1); int(id) <= len(c.colors); id++ {
		names[id] = gen.Next()
	}

	return names
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
