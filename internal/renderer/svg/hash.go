package svg

import "github.com/mrmarble/termsvg/internal/ir"

// FNV-1a parameters shared by every content hash in this package.
const (
	hashOffset = 14695981039346656037
	hashPrime  = 1099511628211
)

func mixHash(h uint64, value int) uint64 {
	//nolint:gosec // convert signed values to stable two's-complement hash words.
	return (h ^ uint64(value)) * hashPrime
}

// attrBits packs the text attributes into one word, in textAttrs order.
func attrBits(attrs ir.CellAttrs) int {
	bits := 0
	for i, set := range attrFlags(attrs) {
		if set {
			bits |= 1 << i
		}
	}
	return bits
}

func hashString(h uint64, s string) uint64 {
	for i := range len(s) {
		h = (h ^ uint64(s[i])) * hashPrime
	}
	return h
}
