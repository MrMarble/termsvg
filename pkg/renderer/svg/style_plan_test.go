package svg

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	stdcolor "image/color"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestLegacyStyleModeIsByteCompatible(t *testing.T) {
	rec := stylePlanRecording()
	config := renderer.DefaultConfig()
	config.ShowCursor = false
	var implicit, explicit bytes.Buffer
	if err := New(config).Render(context.Background(), rec, &implicit); err != nil {
		t.Fatal(err)
	}
	if err := New(config, WithStyleMode(StyleLegacy)).Render(context.Background(), rec, &explicit); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(implicit.Bytes(), explicit.Bytes()) {
		t.Fatal("explicit legacy style changed compatibility output")
	}
}

func TestAutoStyleSelectsNoLargerDeterministicPlan(t *testing.T) {
	rec := stylePlanRecording()
	config := renderer.DefaultConfig()
	config.ShowWindow, config.ShowCursor = false, false
	render := func(style StyleMode) []byte {
		t.Helper()
		var out bytes.Buffer
		if err := New(config, WithStyleMode(style)).Render(context.Background(), rec, &out); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	legacy, first, second := render(StyleLegacy), render(StyleAuto), render(StyleAuto)
	if len(first) > len(legacy) {
		t.Fatalf("auto style bytes = %d, legacy = %d", len(first), len(legacy))
	}
	if !bytes.Equal(first, second) {
		t.Fatal("auto style output is nondeterministic")
	}
	if err := xml.Unmarshal(first, new(any)); err != nil {
		t.Fatalf("auto style output is not XML: %v", err)
	}
	open := string(first[:strings.IndexByte(string(first), '>')+1])
	if strings.Contains(open, " fill=") || !strings.Contains(string(first), `clip-path="url(#clip)" fill="#FFFFFF"`) {
		t.Fatalf("default foreground inheritance escaped the clipped content group: %s", first)
	}
}

func TestAutoStyleUsesClassesOnlyForProfitableBackgroundFills(t *testing.T) {
	rec := stylePlanRecording()
	config := renderer.DefaultConfig()
	config.ShowCursor = false
	var out bytes.Buffer
	if err := New(config, WithStyleMode(StyleAuto)).Render(context.Background(), rec, &out); err != nil {
		t.Fatal(err)
	}
	svg := out.String()
	if !strings.Contains(svg, `fill="#0A141E"`) {
		t.Fatalf("rare background did not use a direct fill: %s", svg)
	}
	if !strings.Contains(svg, `{fill:#28323C}`) {
		t.Fatalf("shared background did not use a fill class: %s", svg)
	}
}

func TestPaintOccurrencesCountOnlySerializedRows(t *testing.T) {
	def := &renderedRow{row: ir.Row{Runs: []ir.TextRun{{Text: "d", EndCol: 1}}}, id: "a"}
	inline := &renderedRow{row: ir.Row{Runs: []ir.TextRun{{Text: "i", EndCol: 1, Attrs: ir.CellAttrs{Bold: true}}}}}
	content := preparedContent{
		rowDefs:       []*renderedRow{def},
		frameRows:     [][]*renderedRow{{def, inline}, {def, inline}},
		frameStateIDs: []string{"_f0", "_f1"},
	}
	c := canvas{
		rec:  experimentalRecording(),
		plan: renderPlan{staticRows: []ir.Row{{Runs: []ir.TextRun{{Text: "s", EndCol: 1}}}}},
	}
	counts := c.countPaintOccurrences(&content)
	if counts.texts[textStyleKey{}] != 2 || counts.texts[textStyleKey{bold: true}] != 2 {
		t.Fatalf("serialized text occurrences = %#v", counts.texts)
	}
}

func TestPreparedStylePlanContainsCandidateScopedRules(t *testing.T) {
	rec := experimentalRecording()
	c := canvas{
		rec: rec, plan: renderPlan{cursorEverVisible: true}, config: *renderer.DefaultConfig(),
	}
	counts := paintOccurrences{
		texts:       map[textStyleKey]int{{bold: true}: 4},
		backgrounds: map[termcolor.ID]int{},
		cursor:      1,
	}
	plan := c.buildStylePlan(styleInheritedAtomic, counts)
	if plan.contentGroupAttributes != ` fill="#FFFFFF"` || plan.textBaseRule == "" || plan.cursorRule == "" ||
		plan.cursorClass == "" || len(plan.rules) == 0 || len(plan.texts) != 1 {
		t.Fatalf("incomplete candidate-scoped style plan: %#v", plan)
	}
}

func TestSharedFillProfitabilityUsesSerializedHexLength(t *testing.T) {
	if sharedFillClassIsProfitable(6, 1, len("fill:#000"), len(` fill="#000"`)) {
		t.Fatal("six short-hex fills do not repay a class rule")
	}
	if !sharedFillClassIsProfitable(7, 1, len("fill:#000"), len(` fill="#000"`)) {
		t.Fatal("seven short-hex fills should repay a class rule")
	}
}

func TestClassNamesMinimizeActualOccurrenceBytesPastZ(t *testing.T) {
	specs := make([]*classSpec, 27)
	for i := range specs {
		specs[i] = &classSpec{key: fmt.Sprintf("rare-%02d", i), occurrences: 1}
	}
	specs[26] = &classSpec{key: "frequent", occurrences: 100}
	assignClassNames(specs)
	for _, spec := range specs {
		if spec.key == "frequent" && spec.name != "a" {
			t.Fatalf("frequent class name = %q, want shortest name a", spec.name)
		}
	}
}

func TestFillPruningReconsidersNamesPastZ(t *testing.T) {
	specs := make([]*classSpec, 0, 27)
	for i := range 25 {
		specs = append(specs, &classSpec{key: fmt.Sprintf("required-%02d", i), declarations: "x", occurrences: 10})
	}
	specs = append(specs,
		&classSpec{key: "short", declarations: "fill:#000", occurrences: 6},
		&classSpec{key: "long", declarations: "fill:#123456", occurrences: 4},
	)
	assignClassNames(specs)
	directBytes := map[string]int{"short": len(` fill="#000"`), "long": len(` fill="#123456"`)}
	specs = pruneUnprofitableFillClasses(specs, directBytes)
	names := make(map[string]string, len(specs))
	for _, spec := range specs {
		names[spec.key] = spec.name
	}
	if names["short"] != "" || names["long"] != "z" {
		t.Fatalf("pruned class names = %#v, want short direct and long class z", names)
	}
}

func TestAutoStyleCostLedgerMatchesSelectedSerialization(t *testing.T) {
	rec := stylePlanRecording()
	config := renderer.DefaultConfig()
	config.ShowWindow, config.ShowCursor = false, false
	plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, 0, config.LoopCount)
	if err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions()
	options.Style = StyleAuto
	candidate, err := prepareCandidate(context.Background(), rec, &plan, config, &options)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.style.scheme == styleLegacy {
		t.Fatal("fixture did not select a profitable auto style plan")
	}
	styleCost := candidate.style.cost
	if candidate.style.styleBytes != styleCost.stylesheet+styleCost.classAttributes+styleCost.directAttributes+
		styleCost.contentGroupAttributes+styleCost.cursorMarkup || candidate.style.styleBytes != styleCost.total {
		t.Fatalf("style bytes = %d, additive ledger = %#v", candidate.style.styleBytes, styleCost)
	}
	if styleCost.stylesheet == 0 || styleCost.classAttributes == 0 || styleCost.directAttributes == 0 ||
		styleCost.contentGroupAttributes == 0 {
		t.Fatalf("style ledger omitted selected encoding components: %#v", styleCost)
	}
	counts := (&canvas{rec: rec, plan: plan}).countPaintOccurrences(&candidate.content)
	if !reflect.DeepEqual(candidate.style.occurrences, counts) {
		t.Fatalf("selected occurrence ledger = %#v, want %#v", candidate.style.occurrences, counts)
	}
	var out bytes.Buffer
	r := New(config, WithStyleMode(StyleAuto))
	if err := r.serializeCandidate(context.Background(), rec, &out, candidate); err != nil {
		t.Fatal(err)
	}
	if int64(out.Len()) != candidate.cost.finalBytes {
		t.Fatalf("serialized bytes = %d, exact cost = %d", out.Len(), candidate.cost.finalBytes)
	}
}

func TestMinifiedAutoStyleCostIsExactAndNoLarger(t *testing.T) {
	rec := stylePlanRecording()
	config := renderer.DefaultConfig()
	config.Minify = true
	measure := func(style StyleMode) int64 {
		t.Helper()
		options := DefaultOptions()
		options.Style = style
		plan, err := buildSemanticPlan(context.Background(), rec, config.ShowCursor, 0, config.LoopCount)
		if err != nil {
			t.Fatal(err)
		}
		candidate, err := prepareCandidate(context.Background(), rec, &plan, config, &options)
		if err != nil {
			t.Fatal(err)
		}
		counter := &countingWriter{}
		r := New(config, WithStyleMode(style))
		if err := writeFinalSVG(counter, true, func(w io.Writer) error {
			return r.serializeCandidate(context.Background(), rec, w, candidate)
		}); err != nil {
			t.Fatal(err)
		}
		if counter.size() != candidate.cost.finalBytes {
			t.Fatalf("minified bytes = %d, exact cost = %d", counter.size(), candidate.cost.finalBytes)
		}
		return counter.size()
	}
	legacy, auto := measure(StyleLegacy), measure(StyleAuto)
	if auto > legacy {
		t.Fatalf("minified auto bytes = %d, legacy = %d", auto, legacy)
	}
}

func stylePlanRecording() *ir.Recording {
	catalog := termcolor.NewCatalog(stdcolor.RGBA{R: 255, G: 255, B: 255, A: 255}, stdcolor.RGBA{A: 255})
	palette := termcolor.Standard()
	rare := catalog.Register(termcolor.FromRGB(10, 20, 30), &palette)
	shared := catalog.Register(termcolor.FromRGB(40, 50, 60), &palette)
	rows := make([]ir.Row, 20)
	for i := range rows {
		bg := shared
		if i == 0 {
			bg = rare
		}
		rows[i] = ir.Row{Y: i, Runs: []ir.TextRun{{
			Text: "styled", EndCol: 6, Attrs: ir.CellAttrs{BG: bg, Bold: true, Italic: true, Underline: true, Dim: true},
		}}}
	}
	return &ir.Recording{
		Width: 10, Height: len(rows), Duration: time.Second, Colors: catalog,
		Frames: []ir.Frame{{Rows: rows}},
		Stats:  ir.Stats{TotalFrames: 1, HasBold: true, HasItalic: true, HasUnderline: true, HasDim: true},
	}
}
