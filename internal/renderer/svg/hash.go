package svg

// FNV-1a style hashing shared by the segment and block content hashes.
const (
	segmentHashOffset = 14695981039346656037
	segmentHashPrime  = 1099511628211
)

func mixSegmentHash(h uint64, value int) uint64 {
	//nolint:gosec // convert signed values to stable two's-complement hash words.
	return (h ^ uint64(value)) * segmentHashPrime
}

func hashString(h uint64, s string) uint64 {
	for i := range len(s) {
		h = (h ^ uint64(s[i])) * segmentHashPrime
	}
	return h
}
