package svg

import "strconv"

// loopCount is renderer.Config.LoopCount: 0 and anything below -1 repeat
// forever, -1 plays once and n > 0 plays n times.
type loopCount int

func (l loopCount) infinite() bool { return l == 0 || l < -1 }

// iterations is the finite repeat count, or "" when the loop is infinite. CSS
// and SMIL spell the infinite case differently ("infinite", "indefinite").
func (l loopCount) iterations() string {
	switch {
	case l.infinite():
		return ""
	case l == -1:
		return "1"
	}
	return strconv.Itoa(int(l))
}

func (l loopCount) css() string {
	if l.infinite() {
		return "infinite"
	}
	return l.iterations()
}

func (l loopCount) smil() string {
	if l.infinite() {
		return "indefinite"
	}
	return l.iterations()
}
