package svg

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
	"github.com/tdewolff/minify/v2"
	msvg "github.com/tdewolff/minify/v2/svg"
)

type writeCountingWriter struct {
	bytes.Buffer
	writes int
}

type failingWriter struct{}

func TestNew(t *testing.T) {
	config := renderer.DefaultConfig()
	r := New(config)

	if r == nil {
		t.Fatal("New() returned nil")
	}
	if r.Format() != "svg" {
		t.Errorf("Format() = %q, want %q", r.Format(), "svg")
	}
	if r.FileExtension() != ".svg" {
		t.Errorf("FileExtension() = %q, want %q", r.FileExtension(), ".svg")
	}
}

func TestRender_EmptyRecording(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := &ir.Recording{
		Width:  80,
		Height: 24,
		Frames: []ir.Frame{},
		Colors: termcolor.NewCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
	}

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)

	if err == nil {
		t.Error("expected error for empty recording, got nil")
	}
}

func (w *writeCountingWriter) Write(p []byte) (int, error) {
	w.writes++
	return w.Buffer.Write(p)
}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestRender_BuffersWrites(t *testing.T) {
	r := New(renderer.DefaultConfig())
	w := &writeCountingWriter{}

	if err := r.Render(context.Background(), createTestRecording(), w); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if w.writes != 1 {
		t.Fatalf("underlying Write() calls = %d, want 1", w.writes)
	}
}

func TestRender_ReturnsWriteError(t *testing.T) {
	err := New(renderer.DefaultConfig()).Render(
		context.Background(), createTestRecording(), failingWriter{},
	)
	if err == nil {
		t.Fatal("Render() ignored the destination write error")
	}
}

func TestRender_BasicStructure(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := createTestRecording()

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Check basic SVG structure
	checks := []string{
		`<svg xmlns="http://www.w3.org/2000/svg"`,
		`</svg>`,
		`<style>`,
		`</style>`,
		`@keyframes k`,
		`<text`,
	}

	for _, check := range checks {
		if !strings.Contains(svg, check) {
			t.Errorf("SVG missing expected element: %q", check)
		}
	}
}

func TestRender_WindowChrome(t *testing.T) {
	config := renderer.DefaultConfig()
	config.ShowWindow = true
	r := New(config)
	rec := createTestRecording()

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Should have rounded rect for window
	if !strings.Contains(svg, `rx="5"`) {
		t.Error("SVG missing window rounded corners")
	}

	// Should have window buttons (circles)
	if !strings.Contains(svg, `<circle`) {
		t.Error("SVG missing window buttons")
	}
}

func TestRender_NoWindowChrome(t *testing.T) {
	config := renderer.DefaultConfig()
	config.ShowWindow = false
	r := New(config)
	rec := createTestRecording()

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Should not have window buttons
	if strings.Contains(svg, `<circle`) {
		t.Error("SVG should not have window buttons when ShowWindow=false")
	}
}

func TestRender_Keyframes(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := createTestRecording()

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Check keyframes exist
	if !strings.Contains(svg, "@keyframes k{") {
		t.Error("SVG missing keyframes animation")
	}

	// Check animation is applied
	if !strings.Contains(svg, "animation:k") {
		t.Error("SVG missing animation style")
	}
}

func TestRender_UsesCompactStepEndTimelines(t *testing.T) {
	rec := createTestRecording()
	rec.Duration = 2 * time.Second
	rec.Frames[1].Time = time.Second
	rec.Frames[0].Cursor = ir.Cursor{Visible: true}
	rec.Frames[1].Cursor = ir.Cursor{Col: 1, Visible: true}

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	for _, want := range []string{
		`@keyframes k{0%{transform:translateX(0px)}50%{transform:translateX(-960px)}100%{transform:translateX(-960px)}}`,
		`@keyframes cursor{0%{transform:translate(0px,0px);visibility:visible}` +
			`50%{transform:translate(12px,0px);visibility:visible}` +
			`100%{transform:translate(12px,0px);visibility:visible}}`,
		`animation:k 2s infinite step-end`,
		`animation:cursor 2s infinite step-end`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG missing compact timeline %q", want)
		}
	}
	if strings.Contains(svg, "steps(1,end)") {
		t.Fatal("SVG retained the longer equivalent timing function")
	}
}

func TestRender_CollapsedSelectorsCompactContentAndCursor(t *testing.T) {
	rec := createTestRecording()
	rec.Duration = time.Duration(1<<63 - 1)
	a := []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "a"}}}}
	b := []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "b"}}}}
	rec.Frames = []ir.Frame{
		{Rows: a, Cursor: ir.Cursor{Visible: true}},
		{Time: rec.Duration - 2*time.Nanosecond, Rows: b, Cursor: ir.Cursor{Col: 1, Visible: true}},
		{Time: rec.Duration - time.Nanosecond, Rows: a, Cursor: ir.Cursor{Visible: true}},
	}

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	for _, forbidden := range []string{"@keyframes k", "animation:k", "@keyframes cursor", "animation:cursor"} {
		if strings.Contains(buf.String(), forbidden) {
			t.Errorf("collapsed content/cursor timeline emitted %q", forbidden)
		}
	}
}

func TestRender_ColorClasses(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := createTestRecording()

	// Register a specific color
	palette := termcolor.Standard()
	redID := rec.Colors.Register(termcolor.FromRGB(255, 0, 0), &palette)

	// Add a frame with that color
	rec.Frames = append(rec.Frames, ir.Frame{
		Time:  2 * time.Second,
		Delay: time.Second,
		Index: 2,
		Rows: []ir.Row{
			{Y: 0, Runs: []ir.TextRun{
				{Text: "Red", StartCol: 0, Attrs: ir.CellAttrs{FG: redID}},
			}},
		},
	})
	rec.Duration = 2 * time.Second

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Should have color class definition
	if !strings.Contains(svg, "#FF0000") {
		t.Error("SVG missing red color definition")
	}

	// Should have class applied to text
	if !strings.Contains(svg, `class="`) {
		t.Error("SVG missing class attribute on text")
	}
}

func TestRender_TextAttributes(t *testing.T) {
	config := renderer.DefaultConfig()
	r := New(config)

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: time.Second,
		Colors: termcolor.NewCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
		Frames: []ir.Frame{
			{
				Time:  0,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{
						{Text: "Bold", StartCol: 0, Attrs: ir.CellAttrs{Bold: true}},
						{Text: "Italic", StartCol: 5, Attrs: ir.CellAttrs{Italic: true}},
						{Text: "Underline", StartCol: 12, Attrs: ir.CellAttrs{Underline: true}},
						{Text: "Dim", StartCol: 22, Attrs: ir.CellAttrs{Dim: true}},
					}},
				},
			},
		},
		Stats: ir.Stats{
			HasBold:      true,
			HasItalic:    true,
			HasUnderline: true,
			HasDim:       true,
		},
	}

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Check attribute classes exist
	if !strings.Contains(svg, ".bold{font-weight:bold}") {
		t.Error("SVG missing bold class")
	}
	if !strings.Contains(svg, ".italic{font-style:italic}") {
		t.Error("SVG missing italic class")
	}
	if !strings.Contains(svg, ".underline{text-decoration:underline}") {
		t.Error("SVG missing underline class")
	}
	if !strings.Contains(svg, ".dim{opacity:0.5}") {
		t.Error("SVG missing dim class")
	}

	// Check classes are applied to text
	if !strings.Contains(svg, `class="bold"`) {
		t.Error("SVG missing bold class on text")
	}
}

//nolint:funlen // recording setup and assertions are clearer together.
func TestRender_BackgroundRectangles(t *testing.T) {
	config := renderer.DefaultConfig()
	r := New(config)

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: time.Second,
		Colors: termcolor.NewCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
		Frames: []ir.Frame{},
	}

	bgPalette := termcolor.Standard()
	blueID := rec.Colors.Register(termcolor.FromRGB(0, 0, 255), &bgPalette)
	redID := rec.Colors.Register(termcolor.FromRGB(255, 0, 0), &bgPalette)
	unusedID := rec.Colors.Register(termcolor.FromRGB(0, 255, 0), &bgPalette)

	rec.Frames = []ir.Frame{
		{
			Time:  0,
			Index: 0,
			Rows: []ir.Row{
				{Y: 1, Runs: []ir.TextRun{
					{Text: "界", StartCol: 1, EndCol: 3, Attrs: ir.CellAttrs{BG: blueID}},
					{Text: "B", StartCol: 3, EndCol: 4, Attrs: ir.CellAttrs{FG: redID, BG: blueID, Bold: true}},
					{Text: "C", StartCol: 4, EndCol: 5, Attrs: ir.CellAttrs{BG: redID}},
					{Text: "D", StartCol: 6, EndCol: 7, Attrs: ir.CellAttrs{BG: redID}},
				}},
			},
		},
	}

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	for _, forbidden := range []string{"<filter", "feFlood", "feComposite", "filter="} {
		if strings.Contains(svg, forbidden) {
			t.Errorf("SVG contains removed background filter markup %q", forbidden)
		}
	}
	for _, want := range []string{
		`<rect class="a" x="12" y="25" width="36" height="25"/>`,
		`<rect class="b" x="48" y="25" width="12" height="25"/>`,
		`<rect class="b" x="72" y="25" width="12" height="25"/>`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG missing background rectangle %q", want)
		}
	}
	if strings.Contains(svg, `.`+rec.Colors.GenerateClassNames()[unusedID]+`{fill:#00FF00}`) {
		t.Error("SVG emitted CSS for an unused registered color")
	}
	if strings.Index(svg, `<rect class="a"`) > strings.Index(svg, `<text x="12"`) {
		t.Error("background rectangle was emitted after foreground text")
	}
}

func TestRender_BackgroundUsesRuneCountWhenEndColUnset(t *testing.T) {
	rec := createTestRecording()
	palette := termcolor.Standard()
	bgID := rec.Colors.Register(termcolor.FromRGB(0, 0, 255), &palette)
	rec.Frames = []ir.Frame{{Rows: []ir.Row{{
		Y: 0, Runs: []ir.TextRun{{Text: "界", StartCol: 2, Attrs: ir.CellAttrs{BG: bgID}}},
	}}}}

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), `<rect class="a" x="24" y="0" width="12" height="25"/>`) {
		t.Fatal("manual TextRun did not use rune-count background fallback")
	}
}

func TestRender_ColoredWhitespace(t *testing.T) {
	rec := createTestRecording()
	palette := termcolor.Standard()
	bgID := rec.Colors.Register(termcolor.FromRGB(0, 0, 255), &palette)
	rec.Frames = []ir.Frame{{Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{
		{Text: "  ", StartCol: 0, EndCol: 2, Attrs: ir.CellAttrs{BG: bgID}},
		{Text: " ", StartCol: 2, EndCol: 3, Attrs: ir.CellAttrs{BG: bgID, Underline: true}},
	}}}}}
	rec.Stats.HasUnderline = true

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	if strings.Count(svg, "<text") != 1 || !strings.Contains(svg, `class="underline"> </text>`) {
		t.Fatalf("colored whitespace text output = %q", svg)
	}
	if !strings.Contains(svg, `<rect class="a" y="0" width="36" height="25"/>`) {
		t.Fatal("colored whitespace backgrounds were not merged")
	}
}

func TestRender_IsDeterministic(t *testing.T) {
	rec := createTestRecording()
	var first bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &first); err != nil {
		t.Fatalf("first Render() error = %v", err)
	}
	for range 3 {
		var next bytes.Buffer
		if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &next); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		if !bytes.Equal(first.Bytes(), next.Bytes()) {
			t.Fatal("repeated rendering produced different SVG bytes")
		}
	}
}

func TestRender_HTMLEscaping(t *testing.T) {
	r := New(renderer.DefaultConfig())

	rec := &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: time.Second,
		Colors: termcolor.NewCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
		Frames: []ir.Frame{
			{
				Time:  0,
				Index: 0,
				Rows: []ir.Row{
					{Y: 0, Runs: []ir.TextRun{
						{Text: "<script>alert('xss')</script>", StartCol: 0},
					}},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	svg := buf.String()

	// Should escape HTML
	if strings.Contains(svg, "<script>") {
		t.Error("SVG contains unescaped script tag")
	}
	if !strings.Contains(svg, "&lt;script&gt;") {
		t.Error("SVG missing escaped script tag")
	}
}

//nolint:funlen,gocognit // normal and minified output share the same regression assertions.
func TestRender_HoistsTextWhitespaceAndEscapesTextNodes(t *testing.T) {
	const inlineText = "  inline middle  "
	const compactInlineText = "inline middle"
	repeatedText := strings.TrimSpace(strings.Repeat("repeated middle ", 8))
	rec := createTestRecording()
	rec.Width = 80
	rec.Frames = []ir.Frame{
		{Time: 0, Rows: []ir.Row{
			{Y: 0, Runs: []ir.TextRun{{Text: "  static  ", StartCol: 2, Attrs: ir.CellAttrs{Underline: true}}}},
			{Y: 1, Runs: []ir.TextRun{{Text: "  " + strings.Repeat("repeated middle ", 8), StartCol: 3}}},
			{Y: 2, Runs: []ir.TextRun{{Text: `<safe>&"'`, StartCol: 5}}},
		}},
		{Time: time.Second, Rows: []ir.Row{
			{Y: 0, Runs: []ir.TextRun{{Text: "  static  ", StartCol: 2, Attrs: ir.CellAttrs{Underline: true}}}},
			{Y: 1, Runs: []ir.TextRun{{Text: "different", StartCol: 3}}},
			{Y: 2, Runs: []ir.TextRun{{Text: `<safe>&"'`, StartCol: 5}}},
			{Y: 3, Runs: []ir.TextRun{{Text: inlineText, StartCol: 4}}},
		}},
		{Time: 2 * time.Second, Rows: []ir.Row{
			{Y: 0, Runs: []ir.TextRun{{Text: "  static  ", StartCol: 2, Attrs: ir.CellAttrs{Underline: true}}}},
			{Y: 1, Runs: []ir.TextRun{{Text: "  " + strings.Repeat("repeated middle ", 8), StartCol: 3}}},
			{Y: 2, Runs: []ir.TextRun{{Text: `<safe>&"'`, StartCol: 5}}},
			{Y: 4, Runs: []ir.TextRun{{Text: "end", StartCol: 1}}},
		}},
	}
	rec.Duration = 2 * time.Second
	rec.Stats.HasUnderline = true

	for _, isMinified := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "minified"}[isMinified], func(t *testing.T) {
			config := renderer.DefaultConfig()
			config.Minify = isMinified
			var buf bytes.Buffer
			if err := New(config).Render(context.Background(), rec, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			svg := buf.String()
			if isMinified {
				m := minify.New()
				m.AddFunc("image/svg+xml", msvg.Minify)
				var output bytes.Buffer
				if err := m.Minify("image/svg+xml", &output, &buf); err != nil {
					t.Fatalf("Minify() error = %v", err)
				}
				svg = strings.ReplaceAll(output.String(), "\u00a0", " ")
			}
			if !isMinified && (strings.Count(svg, `xml:space="preserve"`) != 1 ||
				!strings.Contains(svg, `<svg xmlns="http://www.w3.org/2000/svg" xml:space="preserve"`)) {
				t.Fatalf("xml:space was not inherited from the root SVG: %s", svg)
			}
			if !strings.Contains(svg, `<text x="24" y="20" class="underline">  static  </text>`) ||
				!strings.Contains(svg, `<text id="a" x="60" y="45">`+repeatedText+`</text>`) ||
				!strings.Contains(svg, `<use href="#a"/>`) {
				t.Fatalf("space-preserving static or reused text missing: %s", svg)
			}
			if !strings.Contains(svg, `<text x="60" y="70">&lt;safe`) || strings.Contains(svg, `<safe>`) ||
				strings.Contains(svg, `&#34;`) || strings.Contains(svg, `&#39;`) {
				t.Fatalf("text-node escaping changed quotes or missed XML escapes: %s", svg)
			}
			if !isMinified && !strings.Contains(svg, `&lt;safe&gt;&amp;"'`) {
				t.Fatalf("text-node escaping missed a defensive angle escape: %s", svg)
			}
			inlineAt := strings.Index(svg, `<text x="72" y="95">`+compactInlineText+`</text>`)
			if inlineAt < strings.Index(svg, `</defs>`) {
				t.Fatalf("whitespace-bearing row was not emitted inline: %s", svg)
			}
			if err := xml.Unmarshal([]byte(svg), new(struct{})); err != nil {
				t.Fatalf("SVG is not valid XML: %v", err)
			}
			if got, ok := svgTextAt(svg, "72", "95"); !ok || got != compactInlineText {
				t.Fatalf("inline text after XML parse = %q, found=%t; want %q", got, ok, compactInlineText)
			}
		})
	}
}

func svgTextAt(svg, x, y string) (string, bool) {
	decoder := xml.NewDecoder(strings.NewReader(svg))
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", false
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "text" {
			continue
		}
		var textX, textY string
		for _, attr := range start.Attr {
			if attr.Name.Local == "x" {
				textX = attr.Value
			}
			if attr.Name.Local == "y" {
				textY = attr.Value
			}
		}
		if textX == x && textY == y {
			var text string
			return text, decoder.DecodeElement(&text, &start) == nil
		}
	}
}

func TestRender_LoopCount(t *testing.T) {
	tests := []struct {
		name      string
		loopCount int
		want      string
	}{
		{"infinite", 0, "infinite"},
		{"no loop", -1, "1"},
		{"specific count", 3, "3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := renderer.DefaultConfig()
			config.LoopCount = tt.loopCount
			r := New(config)
			rec := createTestRecording()

			var buf bytes.Buffer
			err := r.Render(context.Background(), rec, &buf)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			svg := buf.String()
			expected := tt.want + " step-end"
			if !strings.Contains(svg, expected) {
				t.Errorf("SVG missing loop count %q", expected)
			}
		})
	}
}

func TestRender_ContextCancellation(t *testing.T) {
	r := New(renderer.DefaultConfig())
	rec := createTestRecording()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	err := r.Render(ctx, rec, &buf)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestCanvas_Dimensions(t *testing.T) {
	config := renderer.DefaultConfig()
	rec := &ir.Recording{
		Width:  80,
		Height: 24,
		Colors: termcolor.NewCatalog(
			color.RGBA{R: 255, G: 255, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 0, A: 255},
		),
	}

	c := &canvas{
		rec:    rec,
		config: *config,
	}

	// Content dimensions
	if c.contentWidth() != 80*ColWidth {
		t.Errorf("contentWidth() = %d, want %d", c.contentWidth(), 80*ColWidth)
	}
	if c.contentHeight() != 24*RowHeight {
		t.Errorf("contentHeight() = %d, want %d", c.contentHeight(), 24*RowHeight)
	}

	// Padded dimensions with window
	config.ShowWindow = true
	c.config = *config
	expectedPaddedHeight := c.contentHeight() + Padding*HeaderSize + Padding // header + bottom padding
	if c.paddedHeight() != expectedPaddedHeight {
		t.Errorf("paddedHeight() with window = %d, want %d", c.paddedHeight(), expectedPaddedHeight)
	}

	// Padded dimensions without window
	config.ShowWindow = false
	c.config = *config
	expectedPaddedHeight = c.contentHeight() + 2*Padding
	if c.paddedHeight() != expectedPaddedHeight {
		t.Errorf("paddedHeight() without window = %d, want %d", c.paddedHeight(), expectedPaddedHeight)
	}
}

// createTestRecording creates a simple recording for testing
func createTestRecording() *ir.Recording {
	colors := termcolor.NewCatalog(
		color.RGBA{R: 255, G: 255, B: 255, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	)

	return &ir.Recording{
		Width:    80,
		Height:   24,
		Duration: time.Second,
		Title:    "Test Recording",
		Colors:   colors,
		Frames: []ir.Frame{
			{
				Time:  0,
				Delay: 0,
				Index: 0,
				Rows: []ir.Row{
					{
						Y: 0,
						Runs: []ir.TextRun{
							{Text: "Hello", StartCol: 0},
							{Text: "World", StartCol: 6},
						},
					},
				},
			},
			{
				Time:  500 * time.Millisecond,
				Delay: 500 * time.Millisecond,
				Index: 1,
				Rows: []ir.Row{
					{
						Y: 0,
						Runs: []ir.TextRun{
							{Text: "Goodbye", StartCol: 0},
						},
					},
				},
			},
		},
		Stats: ir.Stats{
			TotalFrames: 2,
		},
	}
}

// Integration tests using example files

//nolint:funlen // integration test requires multiple setup and verification steps
func TestIntegration_256Colors(t *testing.T) {
	// Find the examples directory (relative to this test file)
	examplesDir := filepath.Join("..", "..", "..", "examples")
	castPath := filepath.Join(examplesDir, "256colors.cast")

	// Skip if example file doesn't exist
	if _, err := os.Stat(castPath); os.IsNotExist(err) {
		t.Skipf("Example file not found: %s", castPath)
	}

	// Load the cast file
	f, err := os.Open(castPath) //nolint:gosec // test file path
	if err != nil {
		t.Fatalf("Failed to open cast file: %v", err)
	}
	defer f.Close()

	cast, err := asciicast.Parse(f)
	if err != nil {
		t.Fatalf("Failed to parse cast file: %v", err)
	}

	// Process through IR
	proc := ir.NewProcessor(ir.DefaultProcessorConfig())
	rec, err := proc.Process(cast)
	if err != nil {
		t.Fatalf("Failed to process cast: %v", err)
	}

	// Verify IR was generated correctly
	if rec.Width != 120 {
		t.Errorf("Recording width = %d, want 120", rec.Width)
	}
	if rec.Height != 42 {
		t.Errorf("Recording height = %d, want 42", rec.Height)
	}
	if len(rec.Frames) == 0 {
		t.Error("Recording has no frames")
	}

	// Render to SVG
	r := New(renderer.DefaultConfig())
	var buf bytes.Buffer
	err = r.Render(context.Background(), rec, &buf)
	if err != nil {
		t.Fatalf("Failed to render SVG: %v", err)
	}

	svg := buf.String()

	// Verify SVG structure
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("Output doesn't start with <svg")
	}
	if !strings.HasSuffix(svg, "</svg>") {
		t.Error("Output doesn't end with </svg>")
	}

	// Verify it contains expected elements for 256 color test
	if !strings.Contains(svg, "@keyframes") {
		t.Error("SVG missing keyframes animation")
	}
	if !strings.Contains(svg, "<style>") {
		t.Error("SVG missing style element")
	}
	if !strings.Contains(svg, "<text") {
		t.Error("SVG missing text elements")
	}

	// Verify multiple color classes were generated (256 color demo should have many)
	if rec.Stats.UniqueColors < 10 {
		t.Errorf("Expected many unique colors for 256color demo, got %d", rec.Stats.UniqueColors)
	}

	if !strings.Contains(svg, `<rect class="`) {
		t.Error("SVG missing background rectangles for 256 color demo")
	}
	for _, forbidden := range []string{"<filter", "feFlood", "feComposite", "filter="} {
		if strings.Contains(svg, forbidden) {
			t.Errorf("SVG contains removed background filter markup %q", forbidden)
		}
	}

	t.Logf("Generated SVG: %d bytes, %d frames, %d unique colors",
		len(svg), rec.Stats.TotalFrames, rec.Stats.UniqueColors)

	outPath := filepath.Join(examplesDir, "256colors.svg")
	//nolint:gosec // test output file needs to be readable
	f, err = os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("Failed to create output SVG file: %v", err)
	}
	defer f.Close()
	_, err = f.WriteString(svg)
	if err != nil {
		t.Fatalf("Failed to write SVG to file: %v", err)
	}
}

func TestRender_PreservesUnderlinedWhitespace(t *testing.T) {
	rec := createTestRecording()
	rec.Frames = []ir.Frame{{
		Rows: []ir.Row{{
			Y:    0,
			Runs: []ir.TextRun{{Text: "   ", Attrs: ir.CellAttrs{Underline: true}}},
		}},
	}}
	rec.Stats.HasUnderline = true

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), `class="underline">   </text>`) {
		t.Fatal("SVG dropped visible underlined spaces")
	}
}

func TestRender_ReusesProfitableRows(t *testing.T) {
	rec := createTestRecording()
	row := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: "ROW:" + strings.Repeat("x", 96)}}}
	rec.Frames = []ir.Frame{
		{Time: 0, Rows: []ir.Row{row}},
		{Time: 500 * time.Millisecond, Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "different"}}}}},
		// The last state repeats the row but differs as a whole, otherwise it
		// would simply reuse the first state's definition.
		{Time: time.Second, Rows: []ir.Row{row, {Y: 1, Runs: []ir.TextRun{{Text: "tail"}}}}},
	}
	rec.Duration = time.Second

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	if strings.Count(svg, "ROW:") != 1 {
		t.Fatalf("repeated row serialized %d times, want 1", strings.Count(svg, "ROW:"))
	}
	if strings.Count(svg, `<use href="#a"/>`) != 2 {
		t.Fatalf("row references = %d, want 2", strings.Count(svg, `<use href="#a"/>`))
	}
}

func TestRender_InlinesUnprofitableRows(t *testing.T) {
	rec := createTestRecording()
	row := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: "x"}}}
	rec.Frames = []ir.Frame{
		{Rows: []ir.Row{row}},
		{Time: 500 * time.Millisecond, Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "y"}}}}},
		{Time: time.Second, Rows: []ir.Row{row}},
	}
	rec.Duration = time.Second

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), `<use href="#`) {
		t.Fatal("short row was reused even though references cost more")
	}
}

func TestCollectRows_InlinesAtExactByteCost(t *testing.T) {
	rec := createTestRecording()
	row := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: strings.Repeat("x", 2)}}}
	rec.Frames = []ir.Frame{
		{Rows: []ir.Row{row}},
		{Time: time.Second, Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "different"}}}}},
		{Time: 2 * time.Second, Rows: []ir.Row{row, {Y: 1, Runs: []ir.TextRun{{Text: "tail"}}}}},
	}
	rec.Duration = 2 * time.Second
	c := &canvas{rec: rec, config: *renderer.DefaultConfig()}

	plan := buildRenderPlan(rec, false)
	c.plan = plan
	_, states := c.contentKeyframes()
	frames, defs := c.collectRows(states)
	if len(frames) != 3 || frames[0][0].id != "" || len(defs) != 0 {
		t.Fatalf("row at exact definition cost was reused: markup=%d id=%q defs=%d",
			len(frames[0][0].svg), frames[0][0].id, len(defs))
	}
}

func TestCollectRows_AccountsForAAIDLength(t *testing.T) {
	rec := createTestRecording()
	rec.Height = 28
	rec.Frames = make([]ir.Frame, 3)
	for _, i := range []int{0, 2} {
		for j := range 26 {
			rec.Frames[i].Rows = append(rec.Frames[i].Rows,
				ir.Row{Y: j, Runs: []ir.TextRun{{Text: strings.Repeat(string(rune('a'+j)), 96)}}})
		}
		rec.Frames[i].Rows = append(rec.Frames[i].Rows,
			ir.Row{Y: 26, Runs: []ir.TextRun{{Text: strings.Repeat("x", 19)}}})
	}
	// The last state must differ from the first as a whole so its rows are
	// interned individually instead of reusing the first state.
	rec.Frames[2].Rows = append(rec.Frames[2].Rows, ir.Row{Y: 27, Runs: []ir.TextRun{{Text: "tail"}}})
	for j := range 27 {
		// Distinct text per row keeps segment sharing out of this measurement.
		text := "different" + strconv.Itoa(j)
		rec.Frames[1].Rows = append(rec.Frames[1].Rows, ir.Row{Y: j, Runs: []ir.TextRun{{Text: text}}})
	}
	rec.Frames[1].Time = time.Second
	rec.Frames[2].Time = 2 * time.Second
	rec.Duration = 2 * time.Second
	// The two identical states would also be shared as row blocks, which
	// replaces the references this measurement inspects.
	c := &canvas{rec: rec, config: *renderer.DefaultConfig(), options: Options{withoutBlockSharing: true}}

	plan := buildRenderPlan(rec, false)
	c.plan = plan
	_, states := c.contentKeyframes()
	frames, defs := c.collectRows(states)
	if len(defs) != 26 || frames[0][25].id != "z" || frames[0][26].id != "" {
		t.Fatalf("aa-length row was reused without a byte saving: markup=%d id=%q defs=%d",
			len(frames[0][26].svg), frames[0][26].id, len(defs))
	}
}

func TestCollectRows_ResolvesHashCollisionsSemantically(t *testing.T) {
	rec := createTestRecording()
	c := &canvas{rec: rec, config: *renderer.DefaultConfig()}
	states := [][]ir.Row{{
		{Y: 0, Runs: []ir.TextRun{{Text: strings.Repeat("a", 96)}}},
		{Y: 1, Runs: []ir.TextRun{{Text: strings.Repeat("b", 96)}}},
		{Y: 0, Runs: []ir.TextRun{{Text: strings.Repeat("a", 96)}}},
	}}

	frames, _ := c.collectRowsWithHash(states, func(ir.Row) uint64 { return 0 })
	if frames[0][0] == frames[0][1] {
		t.Fatal("unequal rows sharing a hash were interned together")
	}
	if frames[0][0] != frames[0][2] {
		t.Fatal("semantically equal rows were not interned together")
	}
}

func TestRender_InlinesUniqueRows(t *testing.T) {
	rec := createTestRecording()
	rec.Frames = []ir.Frame{
		{Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "first unique row"}}}}},
		{Time: time.Second, Rows: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "second unique row"}}}}},
	}
	rec.Duration = time.Second

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), `<use href="#`) {
		t.Fatal("unique rows were emitted as references")
	}
}

func TestRender_PreservesCursorAndTiming(t *testing.T) {
	rec := createTestRecording()
	rec.Frames[0].Cursor = ir.Cursor{Col: 2, Row: 1, Visible: true}
	rec.Frames[1].Cursor = ir.Cursor{Col: 4, Row: 1, Visible: true}

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	for _, want := range []string{
		`0%{transform:translateX(0px)}`,
		`50%{transform:translateX(-960px)}`,
		`0%{transform:translate(24px,25px);visibility:visible}`,
		`50%{transform:translate(48px,25px);visibility:visible}`,
		`<rect class="cursor" width="12" height="25"/>`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG missing preserved frame state %q", want)
		}
	}
	if strings.Count(svg, "@keyframes cursor") != 1 || strings.Count(svg, "animation:cursor") != 1 {
		t.Fatalf("cursor timelines: keyframes=%d animations=%d, want 1 each",
			strings.Count(svg, "@keyframes cursor"), strings.Count(svg, "animation:cursor"))
	}
}

func TestRender_UsesIndependentContentAndCursorLayers(t *testing.T) {
	rec := createTestRecording()
	rec.Width = 10
	rec.Frames[0].Rows = []ir.Row{
		{Y: 0, Runs: []ir.TextRun{{Text: "static"}}},
		{Y: 1, Runs: []ir.TextRun{{Text: "A"}}},
	}
	rec.Frames[1].Rows = []ir.Row{
		{Y: 0, Runs: []ir.TextRun{{Text: "static"}}},
		{Y: 1, Runs: []ir.TextRun{{Text: "B"}}},
	}
	rec.Frames[0].Cursor = ir.Cursor{Col: 1, Visible: true}
	rec.Frames[1].Cursor = ir.Cursor{Col: 2, Visible: true}

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	if strings.Count(svg, `<rect class="cursor"`) != 1 {
		t.Fatalf("cursor rectangles = %d, want 1", strings.Count(svg, `<rect class="cursor"`))
	}
	if strings.Count(svg, ">static</text>") != 1 {
		t.Fatalf("static row copies = %d, want 1", strings.Count(svg, ">static</text>"))
	}
	if !strings.Contains(svg, `translateX(-120px)`) || strings.Contains(svg, `translateX(-160px)`) {
		t.Fatal("content strip did not use the 120px content width")
	}
	staticAt := strings.Index(svg, ">static</text>")
	dynamicAt := strings.Index(svg, ">A</text>")
	cursorAt := strings.Index(svg, `<rect class="cursor"`)
	if staticAt < 0 || dynamicAt < staticAt || cursorAt < dynamicAt {
		t.Fatalf("paint order static=%d dynamic=%d cursor=%d", staticAt, dynamicAt, cursorAt)
	}
	stripAt := strings.Index(svg, `<g style="animation:k`)
	if stripAt < 0 || strings.Contains(svg[:stripAt], ">A</text>") || strings.Contains(svg[:stripAt], ">B</text>") ||
		!strings.Contains(svg[stripAt:], ">A</text>") || !strings.Contains(svg[stripAt:], ">B</text>") {
		t.Fatal("changing A -> B row was not confined to the dynamic strip")
	}
}

func TestRender_OneEffectiveContentStateHasNoFrameStrip(t *testing.T) {
	rec := createTestRecording()
	rec.Frames[1].Rows = rec.Frames[0].Rows
	config := renderer.DefaultConfig()
	config.ShowCursor = false

	var buf bytes.Buffer
	if err := New(config).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	for _, forbidden := range []string{"@keyframes k", "animation:k", `<g transform="translate(0,0)">`} {
		if strings.Contains(svg, forbidden) {
			t.Errorf("single-state SVG contains %q", forbidden)
		}
	}
}

func TestRender_OmitsAnimationsForStaticAndDisabledCursor(t *testing.T) {
	rec := createTestRecording()
	rec.Duration = 0
	rec.Frames = []ir.Frame{{Rows: rec.Frames[0].Rows, Cursor: ir.Cursor{Visible: true}}}
	config := renderer.DefaultConfig()
	config.ShowCursor = false

	var buf bytes.Buffer
	if err := New(config).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	for _, forbidden := range []string{
		"@keyframes k", "animation:k", "@keyframes blink", `.cursor{`, `<rect class="cursor"`,
	} {
		if strings.Contains(svg, forbidden) {
			t.Errorf("static SVG contains %q", forbidden)
		}
	}
}

func TestRender_ZeroDurationUsesFinalContentState(t *testing.T) {
	rec := createTestRecording()
	rec.Duration = 0
	rec.Frames[0].Rows = []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "first"}}}}
	rec.Frames[1].Rows = []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "final"}}}}

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	if strings.Contains(svg, ">first</text>") || !strings.Contains(svg, ">final</text>") ||
		strings.Contains(svg, "animation:k") {
		t.Fatalf("zero-duration content output = %q", svg)
	}
}

func TestRender_ZeroDurationUsesFinalCursorStateWithoutAnimation(t *testing.T) {
	rec := createTestRecording()
	rec.Duration = 0
	rec.Frames[0].Cursor = ir.Cursor{Col: 1, Row: 1, Visible: true}
	rec.Frames[1].Cursor = ir.Cursor{Col: 3, Row: 2, Visible: true}

	var buf bytes.Buffer
	if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	svg := buf.String()
	if !strings.Contains(svg, `<g transform="translate(36,50)" visibility="visible"><rect class="cursor"`) {
		t.Fatal("zero-duration SVG did not render the final cursor state")
	}
	for _, forbidden := range []string{"@keyframes cursor", "animation:cursor", "0.000s"} {
		if strings.Contains(svg, forbidden) {
			t.Errorf("zero-duration cursor SVG contains %q", forbidden)
		}
	}
}

func TestRender_OmitsNeverVisibleCursorAndDoesNotAnimateStaticCursor(t *testing.T) {
	t.Run("never visible", func(t *testing.T) {
		rec := createTestRecording()
		var buf bytes.Buffer
		if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		if strings.Contains(buf.String(), `<rect class="cursor"`) || strings.Contains(buf.String(), "@keyframes blink") {
			t.Fatal("never-visible cursor was serialized")
		}
	})

	t.Run("static", func(t *testing.T) {
		rec := createTestRecording()
		for i := range rec.Frames {
			rec.Frames[i].Cursor = ir.Cursor{Col: 2, Row: 1, Visible: true}
		}
		var buf bytes.Buffer
		if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		if strings.Contains(buf.String(), "@keyframes cursor") {
			t.Fatal("static cursor received position keyframes")
		}
	})
}

func TestRender_OmitsCursorFromDiscardedVisibleStates(t *testing.T) {
	for _, tt := range []struct {
		name     string
		duration time.Duration
		frames   []ir.Frame
	}{
		{
			name:     "same-time visible state is overwritten",
			duration: time.Second,
			frames: []ir.Frame{
				{Cursor: ir.Cursor{Visible: true}},
				{Cursor: ir.Cursor{}},
			},
		},
		{
			name: "zero duration uses final hidden state",
			frames: []ir.Frame{
				{Cursor: ir.Cursor{Visible: true}},
				{Time: time.Second, Cursor: ir.Cursor{}},
			},
		},
		{
			name:     "selector collision discards visible state",
			duration: time.Duration(1<<63 - 1),
			frames: []ir.Frame{
				{Cursor: ir.Cursor{}},
				{Time: time.Duration(1<<63-1) - 2*time.Nanosecond, Cursor: ir.Cursor{Visible: true}},
				{Time: time.Duration(1<<63-1) - time.Nanosecond, Cursor: ir.Cursor{}},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := createTestRecording()
			rec.Duration = tt.duration
			rec.Frames = tt.frames
			var buf bytes.Buffer
			if err := New(renderer.DefaultConfig()).Render(context.Background(), rec, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			for _, forbidden := range []string{"@keyframes blink", `.cursor{`, `<rect class="cursor"`} {
				if strings.Contains(buf.String(), forbidden) {
					t.Errorf("discarded cursor state emitted %q", forbidden)
				}
			}
		})
	}
}
