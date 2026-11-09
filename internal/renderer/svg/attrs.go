package svg

import "github.com/mrmarble/termsvg/internal/ir"

// textAttrs lists the text attributes in one place with every encoding the
// renderer emits for them: the legacy class, the CSS declaration and the
// equivalent presentation attribute. Index i matches attrFlags.
var textAttrs = [4]struct{ class, declaration, attribute string }{
	{"bold", "font-weight:bold", ` font-weight="bold"`},
	{"italic", "font-style:italic", ` font-style="italic"`},
	{"underline", "text-decoration:underline", ` text-decoration="underline"`},
	{"dim", "opacity:0.5", ` opacity="0.5"`},
}

func attrFlags(attrs ir.CellAttrs) [4]bool {
	return [4]bool{attrs.Bold, attrs.Italic, attrs.Underline, attrs.Dim}
}

func (k textStyleKey) attrFlags() [4]bool {
	return [4]bool{k.bold, k.italic, k.underline, k.dim}
}

// legacy reports whether the plan is the compatibility encoding: colour and
// attribute classes, and the zero plan of a canvas that was never styled.
func (p *stylePlan) legacy() bool { return p.scheme == "" || p.scheme == styleLegacy }
