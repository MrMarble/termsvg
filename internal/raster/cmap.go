package raster

import (
	"encoding/binary"
	"errors"
	"io"
	"slices"
	"unicode"
)

// codeRange is an inclusive range of Unicode code points a font may have glyphs
// for. The ranges read from a font's cmap table are a superset of what it
// draws (a range can include a code point that maps to the missing glyph), so a
// match only means the font is worth trying.
type codeRange struct{ lo, hi rune }

const (
	// Bounds on what is read from a font file. System fonts are not trusted:
	// they may be corrupt, and none of the real ones come near these limits.
	maxFontFaces    = 256
	maxFontTables   = 256
	maxCmapSize     = 8 << 20
	maxCmapRanges   = 1 << 20
	sfntHeaderSize  = 12
	tableRecordSize = 16
)

var errBadFont = errors.New("raster: malformed font")

// readFaceCoverage reads which code points each face of a font file covers: one
// entry per face for a TrueType/OpenType font or collection. It reads only the
// font's table directories and cmap tables, never the glyph data.
func readFaceCoverage(src io.ReaderAt, size int64) ([][]codeRange, error) {
	var header [12]byte
	if _, err := src.ReadAt(header[:], 0); err != nil {
		return nil, err
	}

	offsets := []int64{0}
	if string(header[:4]) == "ttcf" {
		n := int(binary.BigEndian.Uint32(header[8:12]))
		if n < 1 || n > maxFontFaces {
			return nil, errBadFont
		}
		table := make([]byte, 4*n)
		if _, err := src.ReadAt(table, 12); err != nil {
			return nil, err
		}
		offsets = offsets[:0]
		for i := range n {
			offsets = append(offsets, int64(binary.BigEndian.Uint32(table[4*i:])))
		}
	}

	faces := make([][]codeRange, 0, len(offsets))
	for _, offset := range offsets {
		ranges, err := readOneFaceCoverage(src, size, offset)
		if err != nil {
			// A face that cannot be read covers nothing; keep the indexes of
			// the others, which a collection's callers rely on.
			ranges = nil
		}
		faces = append(faces, ranges)
	}

	return faces, nil
}

func readOneFaceCoverage(src io.ReaderAt, size, offset int64) ([]codeRange, error) {
	if offset < 0 || offset+sfntHeaderSize > size {
		return nil, errBadFont
	}
	var header [sfntHeaderSize]byte
	if _, err := src.ReadAt(header[:], offset); err != nil {
		return nil, err
	}
	switch binary.BigEndian.Uint32(header[:4]) {
	case 0x00010000, 0x4F54544F, 0x74727565: // TrueType, 'OTTO', 'true'
	default:
		return nil, errBadFont
	}

	numTables := int(binary.BigEndian.Uint16(header[4:6]))
	if numTables < 1 || numTables > maxFontTables {
		return nil, errBadFont
	}
	records := make([]byte, numTables*tableRecordSize)
	if _, err := src.ReadAt(records, offset+sfntHeaderSize); err != nil {
		return nil, err
	}

	for i := range numTables {
		record := records[i*tableRecordSize:]
		if string(record[:4]) != "cmap" {
			continue
		}
		tableOffset := int64(binary.BigEndian.Uint32(record[8:12]))
		tableLength := int64(binary.BigEndian.Uint32(record[12:16]))
		if tableLength < 4 || tableLength > maxCmapSize || tableOffset < 0 || tableOffset+tableLength > size {
			return nil, errBadFont
		}
		cmap := make([]byte, tableLength)
		if _, err := src.ReadAt(cmap, tableOffset); err != nil {
			return nil, err
		}

		return parseCmapRanges(cmap), nil
	}

	return nil, errBadFont
}

// parseCmapRanges returns the merged code point ranges of every Unicode
// subtable of a cmap table that uses format 4 or 12, which cover all the fonts
// in common use. Anything malformed is skipped rather than reported.
func parseCmapRanges(cmap []byte) []codeRange {
	if len(cmap) < 4 {
		return nil
	}
	numSubtables := int(binary.BigEndian.Uint16(cmap[2:4]))
	var ranges []codeRange

	for i := range numSubtables {
		record := 4 + 8*i
		if record+8 > len(cmap) {
			break
		}
		platform := binary.BigEndian.Uint16(cmap[record:])
		encoding := binary.BigEndian.Uint16(cmap[record+2:])
		offset := int64(binary.BigEndian.Uint32(cmap[record+4:]))
		// Unicode platform, or Windows with a Unicode BMP / full repertoire encoding.
		if platform != 0 && (platform != 3 || (encoding != 1 && encoding != 10)) {
			continue
		}
		if offset < 0 || offset+2 > int64(len(cmap)) {
			continue
		}
		subtable := cmap[offset:]
		switch binary.BigEndian.Uint16(subtable) {
		case 4:
			ranges = appendFormat4(ranges, subtable)
		case 12:
			ranges = appendFormat12(ranges, subtable)
		}
		if len(ranges) > maxCmapRanges {
			return mergeRanges(ranges[:maxCmapRanges])
		}
	}

	return mergeRanges(ranges)
}

func appendFormat4(ranges []codeRange, t []byte) []codeRange {
	if len(t) < 14 {
		return ranges
	}
	segCount := int(binary.BigEndian.Uint16(t[6:8])) / 2
	// endCode[], a reserved pad, then startCode[].
	endCodes, startCodes := 14, 14+2*segCount+2
	if startCodes+2*segCount > len(t) {
		return ranges
	}
	for i := range segCount {
		lo := rune(binary.BigEndian.Uint16(t[startCodes+2*i:]))
		hi := rune(binary.BigEndian.Uint16(t[endCodes+2*i:]))
		if lo > hi || lo == 0xFFFF { // 0xFFFF is the terminating segment
			continue
		}
		ranges = append(ranges, codeRange{lo, hi})
	}

	return ranges
}

func appendFormat12(ranges []codeRange, t []byte) []codeRange {
	if len(t) < 16 {
		return ranges
	}
	groups := int64(binary.BigEndian.Uint32(t[12:16]))
	if 16+12*groups > int64(len(t)) {
		groups = (int64(len(t)) - 16) / 12
	}
	for i := range groups {
		g := t[16+12*i:]
		lo, hi := binary.BigEndian.Uint32(g[0:4]), binary.BigEndian.Uint32(g[4:8])
		if lo > hi || hi > unicode.MaxRune {
			continue
		}
		ranges = append(ranges, codeRange{rune(lo), rune(hi)}) //nolint:gosec // both are at most unicode.MaxRune
	}

	return ranges
}

// mergeRanges sorts the ranges and joins those that overlap or touch.
func mergeRanges(ranges []codeRange) []codeRange {
	if len(ranges) == 0 {
		return nil
	}
	slices.SortFunc(ranges, func(a, b codeRange) int { return int(a.lo) - int(b.lo) })
	merged := ranges[:1]
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r.lo <= last.hi+1 {
			last.hi = max(last.hi, r.hi)
			continue
		}
		merged = append(merged, r)
	}

	return slices.Clip(merged)
}

// coversRune reports whether r is inside one of the sorted, merged ranges.
func coversRune(ranges []codeRange, r rune) bool {
	i, found := slices.BinarySearchFunc(ranges, r, func(c codeRange, r rune) int {
		switch {
		case r < c.lo:
			return 1
		case r > c.hi:
			return -1
		}

		return 0
	})

	return found && i < len(ranges)
}
