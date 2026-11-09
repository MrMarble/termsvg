package svg

import "slices"

// The greedy sharing engine behind segment and block sharing. Both factor
// repeated content out of "units" (rows of runs, states of rows) into a
// definition referenced by <use> elements, and differ only in how a range of
// a unit is measured and rendered.

// occurrence is one place a candidate appears: units[unit][start:end], whose
// first run sits at absolute column col and row y.
type occurrence struct {
	unit  int
	start int // inclusive
	end   int // exclusive
	col   int
	y     int
}

// shareCandidate is the engine state of one candidate.
type shareCandidate struct {
	occurrences []occurrence
	order       int
	users       []occurrence
	anchor      occurrence
	elements    int
	bytes       int // markup bytes of the body measured at the anchor
	id          string
	dropped     bool
}

type sharer interface{ share() *shareCandidate }

func (c *shareCandidate) share() *shareCandidate { return c }

// shareUnits measures ranges of units and exposes their claim flags.
type shareUnits interface {
	Bytes(unit, start, end int) int
	Elements(unit, start, end int) int
	Claimed(unit int) []bool
}

// mostFrequentPosition is the occurrence at the most frequent absolute
// position, the first one on ties.
func mostFrequentPosition(users []occurrence) occurrence {
	type position struct{ col, y int }
	counts := make(map[position]int, len(users))
	best, bestCount := position{}, 0
	for _, user := range users {
		at := position{user.col, user.y}
		counts[at]++
		if counts[at] > bestCount {
			best, bestCount = at, counts[at]
		}
	}
	for _, user := range users {
		if user.col == best.col && user.y == best.y {
			return user
		}
	}
	return occurrence{}
}

func chooseAnchor(units shareUnits, c *shareCandidate, users []occurrence) {
	c.anchor = mostFrequentPosition(users)
	c.bytes = units.Bytes(c.anchor.unit, c.anchor.start, c.anchor.end)
	c.elements = units.Elements(c.anchor.unit, c.anchor.start, c.anchor.end)
}

// shareSavings is the exact byte change of sharing the candidate with the
// given identifier length: the definition costs its anchored body plus the
// wrapper, and every user trades its markup for one reference.
func shareSavings(minify bool, units shareUnits, c *shareCandidate, users []occurrence, idLen int) int {
	savings := -(c.bytes + definitionOverhead(idLen, c.elements))
	for _, user := range users {
		dx, dy := (user.col-c.anchor.col)*ColWidth, (user.y-c.anchor.y)*RowHeight
		savings += units.Bytes(user.unit, user.start, user.end) - segmentUseBytes(minify, idLen, dx, dy)
	}
	return savings
}

func definitionOverhead(idLen, elements int) int {
	if elements == 1 {
		return len(` id=""`) + idLen
	}
	return len(`<g id=""></g>`) + idLen
}

func rangeFree(claimed []bool, start, end int) bool {
	for k := start; k < end; k++ {
		if claimed[k] {
			return false
		}
	}
	return true
}

func markRange(claimed []bool, start, end int, value bool) {
	for k := start; k < end; k++ {
		claimed[k] = value
	}
}

func releaseUsers(units shareUnits, users []occurrence) {
	for _, user := range users {
		markRange(units.Claimed(user.unit), user.start, user.end, false)
	}
}

// selectShared greedily accepts candidates by their estimated total saving.
// Every occurrence whose range is still unclaimed becomes a user, so
// overlapping candidates resolve in favour of the larger win. estimatedIDLen
// prices the initial ranking; idLen(accepted) is the identifier length once
// accepted definitions precede the candidate.
func selectShared[C sharer](
	minify bool,
	units shareUnits,
	candidates []C,
	estimatedIDLen int,
	idLen func(accepted int) int,
) []C {
	estimates := make(map[*shareCandidate]int, len(candidates))
	for _, candidate := range candidates {
		c := candidate.share()
		chooseAnchor(units, c, c.occurrences)
		estimates[c] = shareSavings(minify, units, c, c.occurrences, estimatedIDLen)
	}
	slices.SortFunc(candidates, func(a, b C) int {
		ca, cb := a.share(), b.share()
		if estimates[ca] != estimates[cb] {
			return estimates[cb] - estimates[ca]
		}
		return ca.order - cb.order
	})

	accepted := make([]C, 0)
	for _, candidate := range candidates {
		c := candidate.share()
		if estimates[c] <= 0 {
			break
		}
		users := make([]occurrence, 0, len(c.occurrences))
		for _, occ := range c.occurrences {
			claimed := units.Claimed(occ.unit)
			if rangeFree(claimed, occ.start, occ.end) {
				users = append(users, occ)
				markRange(claimed, occ.start, occ.end, true)
			}
		}
		if len(users) >= 2 {
			chooseAnchor(units, c, users)
			if shareSavings(minify, units, c, users, idLen(len(accepted))) > 0 {
				c.users = users
				accepted = append(accepted, candidate)
				continue
			}
		}
		releaseUsers(units, users)
	}
	return accepted
}

// finalizeShared assigns the shortest identifiers to the most referenced
// definitions and drops any candidate whose exact identifier cost makes it
// unprofitable. Dropping only shortens later identifiers, so one pass per drop
// converges quickly.
func finalizeShared[C sharer](minify bool, units shareUnits, accepted []C, ids *xmlIDAllocator) []C {
	slices.SortStableFunc(accepted, func(a, b C) int {
		return len(b.share().users) - len(a.share().users)
	})
	for {
		probe := *ids
		dropped := false
		for _, candidate := range accepted {
			c := candidate.share()
			id := probe.allocate()
			if shareSavings(minify, units, c, c.users, len(id)) <= 0 {
				c.dropped, dropped = true, true
				releaseUsers(units, c.users)
				continue
			}
			c.id = id
		}
		accepted = slices.DeleteFunc(accepted, func(candidate C) bool { return candidate.share().dropped })
		if !dropped {
			*ids = probe
			return accepted
		}
	}
}
