package main

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMeasureUsesActualMinifiedArtifact(t *testing.T) {
	raw := []byte("<svg><text> \u00a0 </text></svg>")
	minified := []byte("<svg><text>   </text></svg>")

	m, err := measure(raw, minified)
	if err != nil {
		t.Fatal(err)
	}
	if m.RawBytes != len(raw) || m.MinifiedBytes != len(minified) {
		t.Fatalf("byte sizes = raw %d, minified %d", m.RawBytes, m.MinifiedBytes)
	}
	if m.GzipBytes != gzipSize(minified) {
		t.Fatalf("gzip bytes = %d, want %d", m.GzipBytes, gzipSize(minified))
	}
}

func TestMeasureCSSSemantics(t *testing.T) {
	raw := []byte(`<svg><style>
/* @keyframes ignored{0%{}} translateX(999px) */ .moving { animation: k 1s; filter: url(#f) }
.idle { animation-delay: 1s; content: "translateX(999px)" }
	@keyframes one { from, 50% { content:"} 99%";
	transform:translateX(+1e2px) } to, 50.0% {
	transform:translate(-2.5e1px,+3e1px) } }
	@keyframes two { from { opacity:0 } to { opacity:1 } }
	</style><g class="moving"><rect filter="url(#f)"/></g><g class="idle"/>
	<g><animateTransform attributeName="transform"/></g>
	<g style="animation:k 1s"><use href="#r"/></g></svg>`)

	m, err := measure(raw, raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.FilterRefs != 2 || m.Keyframes != 2 || m.KeyframeSelectors != 6 || m.DuplicateSelectors != 1 {
		t.Fatalf("filters/keyframes/selectors/duplicates = %d/%d/%d/%d",
			m.FilterRefs, m.Keyframes, m.KeyframeSelectors, m.DuplicateSelectors)
	}
	if m.MaxTranslate != 100 || m.AnimatedGroups != 3 {
		t.Fatalf("max translate/animated groups = %g/%d", m.MaxTranslate, m.AnimatedGroups)
	}
}

func TestMeasureUsesOnlyTranslateXComponent(t *testing.T) {
	raw := []byte(`<svg><style>.x{transform:translate(+1e2px,-900px)
	translate3d(-2.5e2px,800px,700px) translateX(+3e2px)
	translateY(999px)}</style><g transform="translate(-4e2,600)"/></svg>`)
	m, err := measure(raw, raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.MaxTranslate != 400 {
		t.Fatalf("max translate X = %g, want 400", m.MaxTranslate)
	}
}

func TestMeasureReportsStructuralCandidateCosts(t *testing.T) {
	raw := []byte(`<svg width="100" height="50"><defs><g id="b"><text>x</text></g></defs>` +
		`<g><use href="#b"><animate attributeName="href" values="#b;#b"/></use></g>` +
		`<svg width="20" height="10"><g transform="translate(40)"><rect/></g></svg></svg>`)
	m, err := measure(raw, raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.Elements != 10 || m.DefinitionNodes != 3 || m.ActiveNodes != 7 {
		t.Fatalf("nodes = %d/%d/%d", m.Elements, m.DefinitionNodes, m.ActiveNodes)
	}
	if m.TextNodes != 1 || m.RectNodes != 1 || m.GroupNodes != 3 || m.UseNodes != 1 ||
		m.AnimationNodes != 1 || m.AnimatedElements != 1 || m.StateDefinitions != 1 {
		t.Fatalf("breakdown = %#v", m)
	}
	if m.MaxUseDepth != 1 || m.LocalViewportCount != 1 ||
		m.MaxViewportWidth != 20 || m.MaxViewportHeight != 10 {
		t.Fatalf("depth/viewports = %#v", m)
	}
	if m.MaxTranslatedWidth != 60 || m.MaxTranslatedArea != 600 {
		t.Fatalf("translated surface = %d/%d", m.MaxTranslatedWidth, m.MaxTranslatedArea)
	}
}

func TestMeasureCountsOnlySoundActiveAnimations(t *testing.T) {
	raw := []byte(`<svg><style>
.disabled{animation:none}.named-disabled{animation-name:none}
.both.required{animation:k 1s}.ancestor .descendant{animation:k 1s}.enabled{animation:k 1s}
	</style><g class="disabled"/><g class="named-disabled"/><g class="both"/>
	<g class="both required"/><g class="descendant"/>
	<g class="ancestor descendant"/><g class="enabled"/>
	<g style="animation:none"/><g style="animation-name:none"/>
	<g style="animation:k 1s"/><g><animate attributeName="opacity"/></g></svg>`)
	m, err := measure(raw, raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.AnimatedGroups != 4 {
		t.Fatalf("animated groups = %d, want 4", m.AnimatedGroups)
	}
}

func TestMeasureRejectsMalformedKeyframes(t *testing.T) {
	_, err := measure([]byte(`<svg><style>@keyframes k{0%{opacity:0}</style></svg>`), []byte(`<svg/>`))
	if err == nil || !strings.Contains(err.Error(), "unbalanced") {
		t.Fatalf("error = %v, want unbalanced CSS", err)
	}
}

func TestMeasureRejectsMalformedCSSString(t *testing.T) {
	_, err := measure([]byte(`<svg><style>.x{content:"oops}</style></svg>`), []byte(`<svg/>`))
	if err == nil {
		t.Fatal("accepted malformed CSS string")
	}
}

func TestRunWritesSafeAtomicTSV(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "left", "same.svg")
	right := filepath.Join(dir, "right", "same.svg")
	for _, name := range []string{left, right} {
		if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(`<svg><text>ok</text></svg>`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	minified := filepath.Join(dir, "min") + "\tified.svg"
	if err := os.WriteFile(minified, []byte("<svg>\n</svg>"), 0o600); err != nil {
		t.Fatal(err)
	}

	var first, second bytes.Buffer
	args := []string{"-minified", left + "=" + minified, "-minified", right + "=" + minified, left, right}
	if err := run(args, &first); err != nil {
		t.Fatal(err)
	}
	if err := run(args, &second); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatal("TSV or gzip metrics are not deterministic")
	}
	r := csv.NewReader(strings.NewReader(first.String()))
	r.Comma = '\t'
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[1][0] == records[2][0] || !strings.Contains(first.String(), `"`) {
		t.Fatalf("unsafe or collapsed TSV: %q", first.String())
	}

	var partial bytes.Buffer
	missing := filepath.Join(dir, "missing.svg")
	args = []string{"-minified", left + "=" + minified, left, missing}
	if err := run(args, &partial); err == nil || partial.Len() != 0 {
		t.Fatalf("error/partial output = %v/%q", err, partial.String())
	}
}

func gzipSize(data []byte) int {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(data)
	_ = zw.Close()
	return buf.Len()
}
