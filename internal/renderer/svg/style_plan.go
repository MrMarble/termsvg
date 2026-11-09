package svg

import (
	"fmt"
	stdcolor "image/color"
	"slices"
	"strings"

	"github.com/mrmarble/termsvg/internal/color"
	"github.com/mrmarble/termsvg/internal/ir"
	"github.com/mrmarble/termsvg/internal/renderer"
)

type styleScheme string

type textStyleKey struct {
	fg                           color.ID
	bold, italic, underline, dim bool
}

type paintOccurrences struct {
	texts       map[textStyleKey]int
	backgrounds map[color.ID]int
	cursor      int
}

type paintEncoding struct {
	classes    []string
	attributes string
}

type styleRule struct {
	class        string
	declarations string
}

type stylePlan struct {
	scheme                 styleScheme
	contentGroupAttributes string
	textBaseRule           string
	cursorRule             string
	cursorClass            string
	rules                  []styleRule
	texts                  map[textStyleKey]paintEncoding
	backgrounds            map[color.ID]paintEncoding
	occurrences            paintOccurrences
	styleBytes             int64
	cost                   styleCostLedger
}

type styleCostLedger struct {
	stylesheet             int64
	classAttributes        int64
	directAttributes       int64
	contentGroupAttributes int64
	cursorMarkup           int64
	total                  int64
}

type classSpec struct {
	key          string
	declarations string
	occurrences  int
	name         string
}

// stylePlanner derives paint encodings (CSS classes or presentation attributes)
// for a recording. It reads only the recording, the rendering config and the
// semantic plan.
type stylePlanner struct {
	rec    *ir.Recording
	config *renderer.Config
	plan   *semanticPlan
}

// classCollector accumulates class candidates in the order they are offered.
type classCollector struct {
	specs           []*classSpec
	fillDirectBytes map[string]int
}

const (
	styleLegacy             styleScheme = "legacy"
	styleAtomic             styleScheme = "atomic"
	styleComposite          styleScheme = "composite"
	styleInheritedAtomic    styleScheme = "inherited-atomic"
	styleInheritedComposite styleScheme = "inherited-composite"
)

func (sp stylePlanner) legacyStylePlan() stylePlan {
	fg := color.RGBAtoHex(sp.rec.Colors.DefaultForeground())
	plan := stylePlan{
		scheme: styleLegacy,
		textBaseRule: fmt.Sprintf("font-family:%s;font-size:%dpx;fill:%s;white-space:pre",
			sp.config.FontFamily, sp.config.FontSize, fg),
	}
	if sp.plan.cursorEverVisible {
		plan.cursorRule = "fill:" + fg + ";animation:blink 1s step-end infinite"
	}
	return plan
}

func (sp stylePlanner) countPaintOccurrences(content *preparedContent) paintOccurrences {
	counts := paintOccurrences{texts: make(map[textStyleKey]int), backgrounds: make(map[color.ID]int)}
	rows := rowWriter{rec: sp.rec}
	visit := func(row ir.Row) {
		for _, span := range rows.backgroundSpans(row) {
			counts.backgrounds[span.colorID]++
		}
		for _, run := range row.Runs {
			if _, _, ok := compactTextRun(run); ok {
				counts.texts[textStyleKey{run.Attrs.FG, run.Attrs.Bold, run.Attrs.Italic, run.Attrs.Underline, run.Attrs.Dim}]++
			}
		}
	}
	for _, row := range sp.plan.staticRows {
		visit(row)
	}
	visitPainted := func(row *renderedRow) {
		for _, painted := range row.paintRows() {
			visit(painted)
		}
	}
	for _, row := range content.rowDefs {
		visitPainted(row)
	}
	visitRows := func(states [][]*renderedRow) {
		for _, rows := range states {
			for _, row := range rows {
				if row.id == "" {
					visitPainted(row)
				}
			}
		}
	}
	visitRows(content.frameRows)
	for i := range content.bands {
		visitRows(content.bands[i].rows)
		if track := content.bands[i].track; track != nil && len(track.fill) > 0 {
			counts.backgrounds[track.fill[0].state]++
		}
	}
	if sp.plan.cursorEverVisible {
		counts.cursor = 1
	}
	return counts
}

// buildStylePlan chooses the CSS classes (and the attributes that stand in for
// classes that do not pay for themselves) used to paint the content.
func (sp stylePlanner) buildStylePlan(scheme styleScheme, counts paintOccurrences) stylePlan {
	if scheme == styleLegacy {
		return sp.legacyStylePlan()
	}
	plan := stylePlan{
		scheme: scheme, texts: make(map[textStyleKey]paintEncoding), backgrounds: make(map[color.ID]paintEncoding),
		occurrences: counts,
	}
	inherited := scheme == styleInheritedAtomic || scheme == styleInheritedComposite
	composite := scheme == styleComposite || scheme == styleInheritedComposite
	defaultFill := paintHex(sp.rec.Colors.DefaultForeground(), sp.config.Minify)
	sp.setBaseRules(&plan, inherited, defaultFill)

	textKeys := sortedTextKeys(counts.texts)
	backgroundIDs := sortedColorIDs(counts.backgrounds)
	specs, fillDirectBytes := sp.classCandidates(scheme, &counts, textKeys, backgroundIDs)
	if counts.cursor > 0 {
		plan.cursorRule = "animation:blink 1s step-end infinite"
		if !inherited {
			plan.cursorRule = "fill:" + defaultFill + ";" + plan.cursorRule
		}
		specs = append(specs, &classSpec{key: "cursor", declarations: plan.cursorRule, occurrences: 1})
	}

	assignClassNames(specs)
	specs = pruneUnprofitableFillClasses(specs, fillDirectBytes)
	names := make(map[string]string, len(specs))
	for _, spec := range specs {
		names[spec.key] = spec.name
		plan.rules = append(plan.rules, styleRule{class: spec.name, declarations: spec.declarations})
	}
	slices.SortFunc(plan.rules, func(a, b styleRule) int { return strings.Compare(a.class, b.class) })

	sp.encodeBackgrounds(&plan, backgroundIDs, names)
	sp.encodeTexts(&plan, textKeys, names, composite)
	plan.cursorClass = names["cursor"]
	return plan
}

// setBaseRules sets the rule shared by every text element, and for the
// inherited schemes the default fill carried by the content group.
func (sp stylePlanner) setBaseRules(plan *stylePlan, inherited bool, defaultFill string) {
	plan.textBaseRule = fmt.Sprintf("font-family:%s;font-size:%dpx;white-space:pre",
		sp.config.FontFamily, sp.config.FontSize)
	if inherited {
		plan.contentGroupAttributes = ` fill="` + defaultFill + `"`
		return
	}
	plan.textBaseRule = fmt.Sprintf("font-family:%s;font-size:%dpx;fill:%s;white-space:pre",
		sp.config.FontFamily, sp.config.FontSize, defaultFill)
}

func (sp stylePlanner) classCandidates(
	scheme styleScheme,
	counts *paintOccurrences,
	textKeys []textStyleKey,
	backgroundIDs []color.ID,
) (specs []*classSpec, fillDirectBytes map[string]int) {
	cc := &classCollector{fillDirectBytes: make(map[string]int)}
	inherited := scheme == styleInheritedAtomic || scheme == styleInheritedComposite
	atomic := scheme == styleAtomic || scheme == styleInheritedAtomic
	composite := scheme == styleComposite || scheme == styleInheritedComposite

	if atomic {
		addAttributeClasses(cc, counts)
	}
	if atomic || composite {
		sp.addBackgroundClasses(cc, counts, backgroundIDs, inherited)
	}
	if atomic {
		sp.addForegroundClasses(cc, counts, textKeys, inherited)
	}
	if composite {
		sp.addCompositeClasses(cc, counts, textKeys, inherited)
	}
	return cc.specs, cc.fillDirectBytes
}

func (cc *classCollector) add(key, declarations string, occurrences int) {
	if occurrences == 0 {
		return
	}
	cc.specs = append(cc.specs, &classSpec{key: key, declarations: declarations, occurrences: occurrences})
}

// addFill offers a fill class and, for inherited schemes, records what the
// same fill would cost as a direct attribute.
func (cc *classCollector) addFill(key, hex string, occurrences int, inherited bool) {
	cc.add(key, "fill:"+hex, occurrences)
	if inherited {
		cc.fillDirectBytes[key] = len(` fill="` + hex + `"`)
	}
}

// addAttributeClasses offers one class per text attribute (bold, italic, ...).
func addAttributeClasses(cc *classCollector, counts *paintOccurrences) {
	for _, flag := range []struct {
		key, declaration string
		used             func(textStyleKey) bool
	}{
		{"bold", "font-weight:bold", func(k textStyleKey) bool { return k.bold }},
		{"italic", "font-style:italic", func(k textStyleKey) bool { return k.italic }},
		{"underline", "text-decoration:underline", func(k textStyleKey) bool { return k.underline }},
		{"dim", "opacity:0.5", func(k textStyleKey) bool { return k.dim }},
	} {
		occurrences := 0
		for key, count := range counts.texts {
			if flag.used(key) {
				occurrences += count
			}
		}
		cc.add("attr:"+flag.key, flag.declaration, occurrences)
	}
}

func (sp stylePlanner) addBackgroundClasses(
	cc *classCollector, counts *paintOccurrences, ids []color.ID, inherited bool,
) {
	for _, id := range ids {
		hex := paintHex(sp.rec.Colors.Resolved(id), sp.config.Minify)
		cc.addFill(fmt.Sprintf("bg:%d", id), hex, counts.backgrounds[id], inherited)
	}
}

func (sp stylePlanner) addForegroundClasses(
	cc *classCollector, counts *paintOccurrences, textKeys []textStyleKey, inherited bool,
) {
	for _, id := range sp.textColorIDs(textKeys) {
		hex := paintHex(sp.rec.Colors.Resolved(id), sp.config.Minify)
		cc.addFill(fmt.Sprintf("fg:%d", id), hex, textColorOccurrences(counts.texts, id), inherited)
	}
}

// addCompositeClasses offers one class per distinct text style. For inherited
// schemes a style only earns a class when that is smaller than repeating its
// attributes on every element.
func (sp stylePlanner) addCompositeClasses(
	cc *classCollector, counts *paintOccurrences, textKeys []textStyleKey, inherited bool,
) {
	for _, key := range textKeys {
		declarations, attributes := sp.textDeclarations(key)
		if declarations == "" {
			continue
		}
		count := counts.texts[key]
		if !inherited || count*len(attributes) > len(declarations)+count*len(` class="a"`)+4 {
			cc.add("text:"+textStyleSemanticKey(key), declarations, count)
		}
	}
}

// encodeBackgrounds records how each background colour is painted: by class
// when one was kept, otherwise by a direct fill attribute.
func (sp stylePlanner) encodeBackgrounds(plan *stylePlan, ids []color.ID, names map[string]string) {
	for _, id := range ids {
		if name := names[fmt.Sprintf("bg:%d", id)]; name != "" {
			plan.backgrounds[id] = paintEncoding{classes: []string{name}}
			continue
		}
		hex := paintHex(sp.rec.Colors.Resolved(id), sp.config.Minify)
		plan.backgrounds[id] = paintEncoding{attributes: ` fill="` + hex + `"`}
	}
}

// encodeTexts records how each text style is painted: by classes when kept,
// otherwise by direct presentation attributes.
func (sp stylePlanner) encodeTexts(plan *stylePlan, keys []textStyleKey, names map[string]string, composite bool) {
	for _, key := range keys {
		encoding := paintEncoding{}
		if composite {
			if name := names["text:"+textStyleSemanticKey(key)]; name != "" {
				encoding.classes = []string{name}
			} else {
				_, encoding.attributes = sp.textDeclarations(key)
			}
		} else {
			if !sp.rec.Colors.IsDefault(key.fg) {
				if name := names[fmt.Sprintf("fg:%d", key.fg)]; name != "" {
					encoding.classes = append(encoding.classes, name)
				} else {
					hex := paintHex(sp.rec.Colors.Resolved(key.fg), sp.config.Minify)
					encoding.attributes += ` fill="` + hex + `"`
				}
			}
			for _, flag := range []struct {
				set  bool
				name string
			}{
				{key.bold, names["attr:bold"]},
				{key.italic, names["attr:italic"]},
				{key.underline, names["attr:underline"]},
				{key.dim, names["attr:dim"]},
			} {
				if flag.set {
					encoding.classes = append(encoding.classes, flag.name)
				}
			}
		}
		plan.texts[key] = encoding
	}
}

func assignClassNames(specs []*classSpec) {
	slices.SortFunc(specs, func(a, b *classSpec) int {
		// A shorter name saves one byte in its rule and in every occurrence.
		if a.occurrences != b.occurrences {
			return b.occurrences - a.occurrences
		}
		return strings.Compare(a.key, b.key)
	})
	for i, spec := range specs {
		spec.name = compactXMLIDAt(i)
	}
}

func pruneUnprofitableFillClasses(specs []*classSpec, directBytes map[string]int) []*classSpec {
	for {
		removed := false
		for i, spec := range specs {
			direct, optional := directBytes[spec.key]
			if optional && !sharedFillClassIsProfitable(spec.occurrences, len(spec.name), len(spec.declarations), direct) {
				specs = append(specs[:i], specs[i+1:]...)
				assignClassNames(specs)
				removed = true
				break
			}
		}
		if !removed {
			return specs
		}
	}
}

func sharedFillClassIsProfitable(occurrences, nameBytes, declarations, directBytes int) bool {
	return declarations+nameBytes+3+occurrences*(nameBytes+9) < occurrences*directBytes
}

func sortedTextKeys(counts map[textStyleKey]int) []textStyleKey {
	keys := make([]textStyleKey, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b textStyleKey) int {
		return strings.Compare(textStyleSemanticKey(a), textStyleSemanticKey(b))
	})
	return keys
}

func sortedColorIDs(counts map[color.ID]int) []color.ID {
	ids := make([]color.ID, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func textStyleSemanticKey(key textStyleKey) string {
	return fmt.Sprintf("%05d:%t:%t:%t:%t", key.fg, key.bold, key.italic, key.underline, key.dim)
}

func (sp stylePlanner) textColorIDs(keys []textStyleKey) []color.ID {
	seen := make(map[color.ID]bool)
	for _, key := range keys {
		if !sp.rec.Colors.IsDefault(key.fg) {
			seen[key.fg] = true
		}
	}
	return sortedColorIDs(func() map[color.ID]int {
		counts := make(map[color.ID]int, len(seen))
		for id := range seen {
			counts[id] = 1
		}
		return counts
	}())
}

func textColorOccurrences(counts map[textStyleKey]int, id color.ID) int {
	total := 0
	for key, count := range counts {
		if key.fg == id {
			total += count
		}
	}
	return total
}

func (sp stylePlanner) textDeclarations(key textStyleKey) (css, attributes string) {
	declarations := make([]string, 0, 5)
	if !sp.rec.Colors.IsDefault(key.fg) {
		hex := paintHex(sp.rec.Colors.Resolved(key.fg), sp.config.Minify)
		declarations = append(declarations, "fill:"+hex)
		attributes += ` fill="` + hex + `"`
	}
	for _, property := range []struct {
		set                    bool
		declaration, attribute string
	}{
		{key.bold, "font-weight:bold", ` font-weight="bold"`},
		{key.italic, "font-style:italic", ` font-style="italic"`},
		{key.underline, "text-decoration:underline", ` text-decoration="underline"`},
		{key.dim, "opacity:0.5", ` opacity="0.5"`},
	} {
		if property.set {
			declarations = append(declarations, property.declaration)
			attributes += property.attribute
		}
	}
	return strings.Join(declarations, ";"), attributes
}

// paintHex formats a colour as a CSS hex value, using the 3-digit form when
// minifying and it is lossless.
func paintHex(rgba stdcolor.RGBA, minify bool) string {
	hex := color.RGBAtoHex(rgba)
	if minify && hex[1] == hex[2] && hex[3] == hex[4] && hex[5] == hex[6] {
		return "#" + string([]byte{hex[1], hex[3], hex[5]})
	}
	return hex
}

func stylePlanEqual(left, right *stylePlan) bool {
	if left.scheme != right.scheme ||
		left.contentGroupAttributes != right.contentGroupAttributes ||
		left.textBaseRule != right.textBaseRule ||
		left.cursorRule != right.cursorRule ||
		left.cursorClass != right.cursorClass ||
		len(left.rules) != len(right.rules) ||
		len(left.texts) != len(right.texts) ||
		len(left.backgrounds) != len(right.backgrounds) {
		return false
	}
	for i := range left.rules {
		if left.rules[i] != right.rules[i] {
			return false
		}
	}
	for key, value := range left.texts {
		other, ok := right.texts[key]
		if !ok || value.attributes != other.attributes || !slices.Equal(value.classes, other.classes) {
			return false
		}
	}
	for key, value := range left.backgrounds {
		other, ok := right.backgrounds[key]
		if !ok || value.attributes != other.attributes || !slices.Equal(value.classes, other.classes) {
			return false
		}
	}
	return true
}

func styleAttributes(encoding paintEncoding) string {
	class := ""
	if len(encoding.classes) > 0 {
		class = ` class="` + strings.Join(encoding.classes, " ") + `"`
	}
	return class + encoding.attributes
}

func (c *canvas) buildStyleCostLedger() styleCostLedger {
	var stylesheet strings.Builder
	c.writePaintStyles(&stylesheet)
	ledger := styleCostLedger{
		stylesheet:             int64(finalSVGBytes(stylesheet.String(), c.config.Minify)),
		contentGroupAttributes: int64(len(c.style.contentGroupAttributes)),
	}
	addEncoding := func(encoding paintEncoding, occurrences int) {
		if len(encoding.classes) > 0 {
			ledger.classAttributes += int64(occurrences * len(` class="`+strings.Join(encoding.classes, " ")+`"`))
		}
		ledger.directAttributes += int64(occurrences * len(encoding.attributes))
	}
	if c.style.scheme == styleLegacy {
		for key, occurrences := range c.style.occurrences.texts {
			classes := make([]string, 0, 5)
			if !c.rec.Colors.IsDefault(key.fg) {
				classes = append(classes, c.classNames[key.fg])
			}
			for _, class := range []struct {
				set  bool
				name string
			}{{key.bold, "bold"}, {key.italic, "italic"}, {key.underline, "underline"}, {key.dim, "dim"}} {
				if class.set {
					classes = append(classes, class.name)
				}
			}
			addEncoding(paintEncoding{classes: classes}, occurrences)
		}
		for id, occurrences := range c.style.occurrences.backgrounds {
			addEncoding(paintEncoding{classes: []string{c.classNames[id]}}, occurrences)
		}
		if c.style.occurrences.cursor > 0 {
			ledger.cursorMarkup = int64(len(` class="cursor"`))
		}
	} else {
		for key, occurrences := range c.style.occurrences.texts {
			addEncoding(c.style.texts[key], occurrences)
		}
		for id, occurrences := range c.style.occurrences.backgrounds {
			addEncoding(c.style.backgrounds[id], occurrences)
		}
		if c.style.occurrences.cursor > 0 {
			ledger.cursorMarkup = int64(len(` class="` + c.style.cursorClass + `"`))
		}
	}
	ledger.total = ledger.stylesheet + ledger.classAttributes + ledger.directAttributes +
		ledger.contentGroupAttributes + ledger.cursorMarkup
	return ledger
}
