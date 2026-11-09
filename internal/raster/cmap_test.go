package raster

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// coverageComparison compares the cmap index with what the font really draws,
// over every code point of the first three Unicode planes.
type coverageComparison struct {
	have, indexed, falsePositives int
	firstMiss                     rune // a rune the font has but the index lacks; -1 if none
}

var embeddedFonts = map[string][]byte{
	"JetBrainsMono-Regular": jetBrainsMonoTTF,
	"DejaVuSansMono":        dejaVuSansMonoTTF,
	"NotoSansSymbols2":      notoSansSymbols2TTF,
}

func compareCoverage(t *testing.T, data []byte) coverageComparison {
	t.Helper()
	faces, err := readFaceCoverage(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(faces) != 1 {
		t.Fatalf("readFaceCoverage() = %d faces, %v", len(faces), err)
	}
	font, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	var buf sfnt.Buffer
	cmp := coverageComparison{firstMiss: -1}
	for r := rune(0); r < 0x30000; r++ {
		index, _ := font.GlyphIndex(&buf, r)
		covered := coversRune(faces[0], r)
		if index != 0 {
			cmp.have++
			if !covered && cmp.firstMiss < 0 {
				cmp.firstMiss = r
			}
		}
		if covered {
			cmp.indexed++
			if index == 0 {
				cmp.falsePositives++
			}
		}
	}
	if coversRune(faces[0], 0x10FFFD) || coversRune(faces[0], -1) {
		t.Error("index covers runes that no font has")
	}

	return cmp
}

// The index must never miss a rune the font really has, or a usable font would
// never be tried. It may include a few it does not draw, but must stay tight.
func TestReadFaceCoverage_NeverMissesARuneTheFontHas(t *testing.T) {
	for name, data := range embeddedFonts {
		t.Run(name, func(t *testing.T) {
			cmp := compareCoverage(t, data)
			if cmp.have == 0 {
				t.Fatal("the font has no glyphs?")
			}
			if cmp.firstMiss >= 0 {
				t.Fatalf("U+%04X has a glyph but is not in the index", cmp.firstMiss)
			}
			if cmp.falsePositives*20 > cmp.indexed {
				t.Errorf("index has %d runes of which %d have no glyph; it should be close to exact",
					cmp.indexed, cmp.falsePositives)
			}
		})
	}
}

func TestCoversRune(t *testing.T) {
	ranges := []codeRange{{0x20, 0x7e}, {0x100, 0x100}, {0x2800, 0x28ff}}
	for r, want := range map[rune]bool{
		0x1f: false, 0x20: true, 0x7e: true, 0x7f: false, 0xff: false, 0x100: true, 0x101: false,
		0x27ff: false, 0x2800: true, 0x28ff: true, 0x2900: false, 0x10ffff: false,
	} {
		if got := coversRune(ranges, r); got != want {
			t.Errorf("coversRune(U+%04X) = %v, want %v", r, got, want)
		}
	}
	if coversRune(nil, 'a') {
		t.Error("an empty index covers nothing")
	}
}

func TestMergeRanges(t *testing.T) {
	got := mergeRanges([]codeRange{{10, 20}, {1, 5}, {6, 8}, {18, 30}, {40, 41}})
	want := []codeRange{{1, 8}, {10, 30}, {40, 41}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if mergeRanges(nil) != nil {
		t.Error("merging nothing should give nothing")
	}
}

// u16 and u32 convert fixture sizes, which are tiny, to the widths a font stores.
func u16(n int) uint16 { return uint16(n) } //nolint:gosec // fixture sizes are tiny

func u32(n int) uint32 { return uint32(n) } //nolint:gosec // fixture sizes are tiny

// cmapWith builds a cmap table with the given subtables, each as
// {platformID, encodingID, subtable bytes}.
func cmapWith(subtables ...[]byte) []byte {
	header := make([]byte, 4, 4+8*len(subtables))
	binary.BigEndian.PutUint16(header[2:], u16(len(subtables)))
	records := make([]byte, 0, 8*len(subtables))
	var bodies []byte
	next := 4 + 8*len(subtables)
	for _, st := range subtables {
		platform, encoding, body := binary.BigEndian.Uint16(st[0:]), binary.BigEndian.Uint16(st[2:]), st[4:]
		record := make([]byte, 8)
		binary.BigEndian.PutUint16(record[0:], platform)
		binary.BigEndian.PutUint16(record[2:], encoding)
		binary.BigEndian.PutUint32(record[4:], u32(next+len(bodies)))
		records = append(records, record...)
		bodies = append(bodies, body...)
	}
	return append(append(header, records...), bodies...)
}

func subtable(platform, encoding uint16, body []byte) []byte {
	head := make([]byte, 0, 4+len(body))
	head = binary.BigEndian.AppendUint16(head, platform)
	head = binary.BigEndian.AppendUint16(head, encoding)
	return append(head, body...)
}

func format12(groups ...[2]uint32) []byte {
	body := make([]byte, 16, 16+12*len(groups))
	binary.BigEndian.PutUint16(body[0:], 12)
	binary.BigEndian.PutUint32(body[12:], u32(len(groups)))
	for _, g := range groups {
		entry := make([]byte, 12)
		binary.BigEndian.PutUint32(entry[0:], g[0])
		binary.BigEndian.PutUint32(entry[4:], g[1])
		body = append(body, entry...)
	}
	return body
}

func format4(segments ...[2]uint16) []byte {
	n := len(segments)
	body := make([]byte, 14+8*n+2)
	binary.BigEndian.PutUint16(body[0:], 4)
	binary.BigEndian.PutUint16(body[6:], u16(2*n))
	for i, seg := range segments {
		binary.BigEndian.PutUint16(body[14+2*i:], seg[1])       // endCode
		binary.BigEndian.PutUint16(body[14+2*n+2+2*i:], seg[0]) // startCode
	}
	return body
}

func TestParseCmapRanges(t *testing.T) {
	tests := []struct {
		name string
		cmap []byte
		want []codeRange
	}{
		{
			name: "format 12", cmap: cmapWith(subtable(3, 10, format12([2]uint32{0x41, 0x5a}, [2]uint32{0x1f600, 0x1f64f}))),
			want: []codeRange{{0x41, 0x5a}, {0x1f600, 0x1f64f}},
		},
		{
			name: "format 4 drops the terminating segment",
			cmap: cmapWith(subtable(3, 1, format4([2]uint16{0x20, 0x7e}, [2]uint16{0xffff, 0xffff}))),
			want: []codeRange{{0x20, 0x7e}},
		},
		{
			name: "unicode platform", cmap: cmapWith(subtable(0, 3, format4([2]uint16{0x61, 0x7a}))),
			want: []codeRange{{0x61, 0x7a}},
		},
		{
			name: "subtables are merged", cmap: cmapWith(
				subtable(3, 1, format4([2]uint16{0x20, 0x7e})), subtable(3, 10, format12([2]uint32{0x7f, 0x9f}))),
			want: []codeRange{{0x20, 0x9f}},
		},
		{
			name: "symbol and legacy encodings are ignored", cmap: cmapWith(
				subtable(3, 0, format4([2]uint16{0x20, 0x7e})), subtable(1, 0, format4([2]uint16{0x20, 0x7e}))),
			want: nil,
		},
		{name: "inverted group is skipped", cmap: cmapWith(subtable(3, 10, format12([2]uint32{0x50, 0x40}))), want: nil},
		{
			name: "beyond Unicode is skipped", cmap: cmapWith(subtable(3, 10, format12([2]uint32{0x10ffff, 0x110000}))),
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCmapRanges(tt.cmap)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestParseCmapRanges_ToleratesMalformedTables(t *testing.T) {
	huge := cmapWith(subtable(3, 10, format12([2]uint32{1, 2})))
	binary.BigEndian.PutUint32(huge[4+8+12:], 0xffffffff) // claims 4 billion groups

	badOffset := cmapWith(subtable(3, 10, format12([2]uint32{1, 2})))
	binary.BigEndian.PutUint32(badOffset[4+4:], 0xfffffff0)

	manyTables := make([]byte, 4)
	binary.BigEndian.PutUint16(manyTables[2:], 0xffff) // far more records than bytes

	for name, cmap := range map[string][]byte{
		"nil": nil, "one byte": {1}, "header only": {0, 0, 0, 1}, "claims many tables": manyTables,
		"claims 4 billion groups": huge, "subtable offset past the end": badOffset,
		"truncated format 4": cmapWith(subtable(3, 1, format4([2]uint16{1, 2})))[:20],
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("panicked: %v", p)
				}
			}()
			parseCmapRanges(cmap) // must return, whatever it finds
		})
	}
}

// Whatever is on disk, reading it must never panic or hang.
func TestReadFaceCoverage_SurvivesCorruptFiles(t *testing.T) {
	data := dejaVuSansMonoTTF
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic test input

	cases := map[string][]byte{"empty": nil, "not a font": []byte("this is not a font at all, just text")}
	for _, cut := range []int{1, 4, 11, 12, 13, 28, 100, 500, 4000, len(data) / 2, len(data) - 1} {
		cases["truncated at "+string(rune('a'+cut%26))] = data[:cut]
	}
	for i := range 40 {
		corrupt := bytes.Clone(data[:8192]) // the table directory and the first tables
		for range 8 {
			corrupt[rng.Intn(len(corrupt))] = byte(rng.Uint32() & 0xff)
		}
		cases["bit rot "+string(rune('a'+i%26))+string(rune('a'+i/26))] = corrupt
	}
	// A header that claims an absurd number of tables or faces.
	absurd := bytes.Clone(data[:200])
	binary.BigEndian.PutUint16(absurd[4:], 0xffff)
	cases["absurd table count"] = absurd
	collection := append([]byte("ttcf\x00\x01\x00\x00\xff\xff\xff\xff"), make([]byte, 64)...)
	cases["absurd face count"] = collection

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("panicked: %v", p)
				}
			}()
			if _, err := readFaceCoverage(bytes.NewReader(content), int64(len(content))); err != nil {
				t.Logf("rejected as expected: %v", err)
			}
		})
	}
}

// makeCollection packs fonts into a .ttc, moving each table offset to where
// the table now lives in the collection file.
func makeCollection(t *testing.T, fonts ...[]byte) []byte {
	t.Helper()
	header := make([]byte, 12+4*len(fonts))
	copy(header, "ttcf")
	binary.BigEndian.PutUint32(header[4:], 0x00010000)
	binary.BigEndian.PutUint32(header[8:], u32(len(fonts)))

	out := header
	for i, font := range fonts {
		base := u32(len(out))
		binary.BigEndian.PutUint32(out[12+4*i:], base)
		shifted := bytes.Clone(font)
		numTables := int(binary.BigEndian.Uint16(shifted[4:]))
		for j := range numTables {
			record := shifted[12+16*j:]
			binary.BigEndian.PutUint32(record[8:], binary.BigEndian.Uint32(record[8:])+base)
		}
		out = append(out, shifted...)
	}
	return out
}

func TestReadFaceCoverage_Collection(t *testing.T) {
	ttc := makeCollection(t, dejaVuSansMonoTTF, notoSansSymbols2TTF)

	// The collection must be a valid font collection for the real parser too.
	collection, err := opentype.ParseCollection(ttc)
	if err != nil || collection.NumFonts() != 2 {
		t.Fatalf("test fixture is not a collection: %v", err)
	}

	faces, err := readFaceCoverage(bytes.NewReader(ttc), int64(len(ttc)))
	if err != nil || len(faces) != 2 {
		t.Fatalf("readFaceCoverage() = %d faces, %v", len(faces), err)
	}
	if !coversRune(faces[0], 'ȧ') || coversRune(faces[1], 'ȧ') {
		t.Error("face 0 (DejaVu) should cover ȧ and face 1 (Noto Symbols 2) should not")
	}
	if coversRune(faces[0], '⠿') && !coversRune(faces[1], '⠿') {
		t.Error("Braille belongs to face 1")
	}
	if !coversRune(faces[1], '⠿') {
		t.Error("face 1 (Noto Symbols 2) should cover Braille")
	}
}
