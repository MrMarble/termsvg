package svg

import (
	"testing"

	"github.com/mrmarble/termsvg/pkg/ir"
)

func TestCompactTextRunTrimsOnlyInertASCIISpaces(t *testing.T) {
	run := ir.TextRun{Text: "  value  ", StartCol: 3}

	text, startCol, ok := compactTextRun(run)

	if !ok || text != "value" || startCol != 5 {
		t.Fatalf("compactTextRun() = %q, %d, %v; want value, 5, true", text, startCol, ok)
	}
}

func TestCompactTextRunPreservesDecoratedAndUnicodeWhitespace(t *testing.T) {
	tests := []struct {
		name string
		run  ir.TextRun
		want string
	}{
		{name: "underlined", run: ir.TextRun{Text: "  ", Attrs: ir.CellAttrs{Underline: true}}, want: "  "},
		{name: "nbsp", run: ir.TextRun{Text: "\u00a0"}, want: "\u00a0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, _, ok := compactTextRun(tt.run)
			if !ok || text != tt.want {
				t.Fatalf("compactTextRun() = %q, _, %v; want %q, true", text, ok, tt.want)
			}
		})
	}
}

func TestCompactTextRunOmitsBlankUndecoratedRun(t *testing.T) {
	if _, _, ok := compactTextRun(ir.TextRun{Text: "   "}); ok {
		t.Fatal("compactTextRun() retained an undecorated ASCII-space-only run")
	}
}

func TestCompactXMLIDUsesShortDeterministicNames(t *testing.T) {
	for index, want := range []string{
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n",
		"o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "aa", "ab",
	} {
		if got := compactXMLID(index); got != want {
			t.Fatalf("compactXMLID(%d) = %q; want %q", index, got, want)
		}
	}
}

func TestFinalSVGBytesAccountsForPostMinifyNBSPTransform(t *testing.T) {
	const value = "a\u00a0b\u00a0c"
	if got, want := finalSVGBytes(value, true), len("a b c"); got != want {
		t.Fatalf("finalSVGBytes() = %d; want %d", got, want)
	}
	if got, want := finalSVGBytes(value, false), len(value); got != want {
		t.Fatalf("unminified finalSVGBytes() = %d; want %d", got, want)
	}
}

func TestAddElementIDAvoidsRedundantGroup(t *testing.T) {
	tests := map[string]string{
		`<text y="20">x</text>`:            `<text id="a" y="20">x</text>`,
		`<rect y="0" width="12"/>`:         `<rect id="a" y="0" width="12"/>`,
		`<use href="#b"/>`:                 `<use id="a" href="#b"/>`,
		`<text id="b">x</text>`:            `<g id="a"><text id="b">x</text></g>`,
		`<text>one</text><text>two</text>`: `<text id="a">one</text><text>two</text>`,
	}
	for input, want := range tests {
		if got := addElementID(input, "a"); got != want {
			t.Fatalf("addElementID(%q) = %q; want %q", input, got, want)
		}
	}
}
