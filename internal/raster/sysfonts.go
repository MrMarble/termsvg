package raster

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// systemFonts finds installed fonts that cover a rune. It reads only the
// cmap table of each font, once, the first time a rune needs it, and keeps
// nothing open: a machine can have hundreds of fonts, and the callers open just
// the one they choose. It is safe for concurrent use.
type systemFonts struct {
	preferred []string // searched first: the user's own directories
	dirs      []string // the platform's font directories
	once      sync.Once
	fonts     []systemFont
}

// systemFont is one face of an installed font file and the code points it may cover.
type systemFont struct {
	path      string
	face      int  // index in a collection (.ttc); 0 for a single font
	preferred bool // found in one of the user's own directories
	ranges    []codeRange
}

// systemFontRef names a face to try for a rune.
type systemFontRef struct {
	path string
	face int
}

// fontPathEnv lists extra font directories, separated like PATH. They are
// searched before the system ones, so a font placed there wins.
const fontPathEnv = "TERMSVG_FONT_PATH"

// newSystemFonts indexes fonts from the given directories. Fonts in preferred
// directories are tried before any in dirs, whatever their names.
func newSystemFonts(preferred, dirs []string) *systemFonts {
	return &systemFonts{preferred: preferred, dirs: dirs}
}

// defaultFontDirs returns the directories named in TERMSVG_FONT_PATH, which are
// preferred, and the ones the platform keeps fonts in.
func defaultFontDirs() (preferred, dirs []string) {
	preferred = filepath.SplitList(os.Getenv(fontPathEnv))
	home, _ := os.UserHomeDir()

	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs, "/System/Library/Fonts", "/System/Library/Fonts/Supplemental", "/Library/Fonts")
		if home != "" {
			dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
		}
	case "windows":
		if windir := os.Getenv("WINDIR"); windir != "" {
			dirs = append(dirs, filepath.Join(windir, "Fonts"))
		}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			dirs = append(dirs, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
		}
	default: // Linux and the BSDs
		dirs = append(dirs, "/usr/share/fonts", "/usr/local/share/fonts")
		if data := os.Getenv("XDG_DATA_HOME"); data != "" {
			dirs = append(dirs, filepath.Join(data, "fonts"))
		} else if home != "" {
			dirs = append(dirs, filepath.Join(home, ".local", "share", "fonts"))
		}
		if home != "" {
			dirs = append(dirs, filepath.Join(home, ".fonts"))
		}
	}

	empty := func(d string) bool { return d == "" }

	return slices.DeleteFunc(preferred, empty), slices.DeleteFunc(dirs, empty)
}

// candidates returns the installed faces that may have a glyph for r, best
// first. A face in the list can still turn out not to draw it.
func (s *systemFonts) candidates(r rune) []systemFontRef {
	s.once.Do(s.build)

	var refs []systemFontRef
	for i := range s.fonts {
		if coversRune(s.fonts[i].ranges, r) {
			refs = append(refs, systemFontRef{s.fonts[i].path, s.fonts[i].face})
		}
	}

	return refs
}

func (s *systemFonts) build() {
	seen := make(map[string]bool)
	walk := func(dirs []string, preferred bool) {
		for _, dir := range dirs {
			_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !isFontFile(path) || seen[path] {
					return nil //nolint:nilerr // unreadable entries are skipped, not fatal
				}
				seen[path] = true
				for _, font := range readSystemFont(path) {
					font.preferred = preferred
					s.fonts = append(s.fonts, font)
				}

				return nil
			})
		}
	}
	walk(s.preferred, true)
	walk(s.dirs, false)

	// The user's directories first, then the best suited faces, then by path so
	// the choice never depends on the order the file system listed things in.
	slices.SortStableFunc(s.fonts, func(a, b systemFont) int {
		if a.preferred != b.preferred {
			if a.preferred {
				return -1
			}

			return 1
		}
		if pa, pb := fontPriority(a.path), fontPriority(b.path); pa != pb {
			return pa - pb
		}
		if c := strings.Compare(a.path, b.path); c != 0 {
			return c
		}

		return a.face - b.face
	})
}

func isFontFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf", ".ttc", ".otc":
		return true
	}

	return false
}

// readSystemFont reads the coverage of every face in a font file. Files that
// cannot be read or parsed are skipped.
func readSystemFont(path string) []systemFont {
	file, err := os.Open(path) //nolint:gosec // path comes from a font directory walk
	if err != nil {
		return nil
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil
	}
	faces, err := readFaceCoverage(file, info.Size())
	if err != nil {
		return nil
	}

	fonts := make([]systemFont, 0, len(faces))
	for i, ranges := range faces {
		if len(ranges) > 0 {
			fonts = append(fonts, systemFont{path: path, face: i, ranges: ranges})
		}
	}

	return fonts
}

// fontPriority ranks fonts by how well they suit a terminal: plain upright
// monospaced faces first, then other plain faces, then styled ones. The file
// name is the only cheap signal available without opening every font.
func fontPriority(path string) int {
	name := strings.ToLower(filepath.Base(path))
	priority := 2
	if strings.Contains(name, "mono") || strings.Contains(name, "code") || strings.Contains(name, "courier") {
		priority = 0
	}
	for _, style := range []string{
		"bold", "italic", "oblique", "light", "thin", "black", "heavy", "medium",
		"semi", "demi", "condensed", "narrow", "extra", "ultra",
	} {
		if strings.Contains(name, style) {
			priority += 4

			break
		}
	}

	return priority
}
