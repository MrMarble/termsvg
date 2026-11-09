package raster

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/opentype"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func paths(refs []systemFontRef) []string {
	out := make([]string, len(refs))
	for i, ref := range refs {
		out[i] = filepath.Base(ref.path)
	}
	return out
}

func TestSystemFonts_FindsFontsThatCoverARune(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "nested", "deeper", "DejaVuSansMono.ttf"), dejaVuSansMonoTTF)
	writeFile(t, filepath.Join(dir, "JetBrainsMono-Regular.ttf"), jetBrainsMonoTTF)
	writeFile(t, filepath.Join(dir, "NotoSansSymbols2-Regular.OTF"), notoSansSymbols2TTF) // extension is case-insensitive
	writeFile(t, filepath.Join(dir, "notes.txt"), []byte("not a font"))
	writeFile(t, filepath.Join(dir, "broken.ttf"), []byte("not a font either"))
	writeFile(t, filepath.Join(dir, "empty.ttf"), nil)
	if err := os.Mkdir(filepath.Join(dir, "empty-dir"), 0o750); err != nil {
		t.Fatal(err)
	}

	sys := newSystemFonts([]string{dir, filepath.Join(dir, "does-not-exist")})

	tests := []struct {
		name string
		r    rune
		want []string
	}{
		{name: "latin extended", r: 'ȧ', want: []string{"DejaVuSansMono.ttf"}},
		{name: "braille", r: '⠿', want: []string{"NotoSansSymbols2-Regular.OTF"}},
		{name: "shared by two fonts", r: 'a', want: []string{"DejaVuSansMono.ttf", "JetBrainsMono-Regular.ttf"}},
		{name: "nowhere", r: '\U0010FFFD', want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths(sys.candidates(tt.r))
			slices.Sort(got)
			slices.Sort(tt.want)
			if !slices.Equal(got, tt.want) {
				t.Errorf("candidates(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

func TestSystemFonts_PrefersMonospacedThenPlainFaces(t *testing.T) {
	dir := t.TempDir()
	// All three cover 'a'; the names are what tells them apart.
	names := []string{"Zeta-Bold.ttf", "Alpha.ttf", "Beta Mono.ttf", "Gamma-Italic.ttf", "Delta Mono Bold.ttf"}
	for _, name := range names {
		writeFile(t, filepath.Join(dir, name), dejaVuSansMonoTTF)
	}

	want := []string{"Beta Mono.ttf", "Alpha.ttf", "Delta Mono Bold.ttf", "Gamma-Italic.ttf", "Zeta-Bold.ttf"}
	for range 5 { // a fresh index each time: the order must not depend on listing order
		got := paths(newSystemFonts([]string{dir}).candidates('a'))
		if !slices.Equal(got, want) {
			t.Fatalf("candidates = %v, want %v", got, want)
		}
	}
}

func TestSystemFonts_ListsAFileOnceWhenDirectoriesOverlap(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub", "DejaVuSansMono.ttf"), dejaVuSansMonoTTF)
	sys := newSystemFonts([]string{dir, filepath.Join(dir, "sub"), dir})
	if got := paths(sys.candidates('ȧ')); len(got) != 1 {
		t.Errorf("candidates = %v, want the file once", got)
	}
}

func TestSystemFonts_CollectionFaces(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pair.ttc"), makeCollection(t, dejaVuSansMonoTTF, notoSansSymbols2TTF))

	sys := newSystemFonts([]string{dir})
	got := sys.candidates('⠿')
	if len(got) != 1 || got[0].face != 1 {
		t.Fatalf("candidates(⠿) = %+v, want face 1 of the collection", got)
	}
	if got := sys.candidates('ȧ'); len(got) != 1 || got[0].face != 0 {
		t.Fatalf("candidates(ȧ) = %+v, want face 0 of the collection", got)
	}

	// The reference must open the right face.
	otf, closer, err := openSystemFont(sys.candidates('⠿')[0])
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	if !hasGlyph(t, otf, '⠿') || hasGlyph(t, otf, 'ȧ') {
		t.Error("opened the wrong face of the collection")
	}
}

func hasGlyph(t *testing.T, f *opentype.Font, r rune) bool {
	t.Helper()
	index, err := f.GlyphIndex(nil, r)
	return err == nil && index != 0
}

func TestSystemFonts_IsSafeForConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "DejaVuSansMono.ttf"), dejaVuSansMonoTTF)
	sys := newSystemFonts([]string{dir})

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if got := sys.candidates('ȧ'); len(got) != 1 {
				t.Errorf("candidates = %v", got)
			}
		})
	}
	wg.Wait()
}

func TestDefaultFontDirs(t *testing.T) {
	extra1, extra2 := t.TempDir(), t.TempDir()
	t.Setenv(fontPathEnv, extra1+string(os.PathListSeparator)+extra2)

	dirs := defaultFontDirs()
	if len(dirs) < 3 || dirs[0] != extra1 || dirs[1] != extra2 {
		t.Fatalf("dirs = %v, want %s variable's directories first", dirs, fontPathEnv)
	}
	for _, dir := range dirs {
		if dir == "" {
			t.Error("empty directory in the list")
		}
	}

	t.Setenv(fontPathEnv, "")
	for _, dir := range defaultFontDirs() {
		if dir == extra1 || dir == "" {
			t.Errorf("unexpected directory %q", dir)
		}
	}
	if runtime.GOOS == "linux" {
		if dirs := defaultFontDirs(); !slices.Contains(dirs, "/usr/share/fonts") {
			t.Errorf("dirs = %v, want /usr/share/fonts", dirs)
		}
	}
}

func TestFontPriority(t *testing.T) {
	order := []string{
		"DejaVuSansMono.ttf", "FiraCode-Regular.ttf", // monospaced
		"NotoSans-Regular.ttf", "Symbola.ttf", // plain
		"DejaVuSansMono-Bold.ttf", "Foo-Italic.otf", "Bar-Light.ttf", // styled
	}
	last := -1
	for _, name := range order {
		p := fontPriority("/fonts/" + strings.ToUpper(name[:1]) + name[1:])
		if p < last {
			t.Errorf("%s ranks %d, ahead of an earlier entry (%d)", name, p, last)
		}
		last = p
	}
}
