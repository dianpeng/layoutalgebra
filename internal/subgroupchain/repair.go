package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
	"github.com/dianpeng/layoutalgebra/internal/common"
)

// --------------------------------------------------------------------------------------
//
// Coverage repair engine — universal component
//
// Raises the family's SUFFIX occupancy of the last dimension to >= a target
// window, by piece reordering (when the freedom is already in the dim) or by
// a divisor-search GCD draw from the donor dimensions (when it is not). All
// transforms are bijections: every dim keeps its extent and every variable
// is consumed exactly once.
//
// Feasibility theorem (machine-validated): the draw succeeds iff
//
//	gcd(E, V_ext) >= ceil(window / R')
//
// where E is the non-relevant occupancy, R' the family occupancy and V_ext
// the family freedom outside the last dim. The greedy GCD chain is complete
// and order-independent, so a stall is a certificate that no swap-based
// scheme exists.
//
// --------------------------------------------------------------------------------------

// pendingReplacement records a rewrite to apply to a donor leaf after the
// whole draw succeeded (transactional: an infeasible draw leaves the layout
// untouched).
type pendingReplacement struct {
	htuple       *HTuple
	idx          int
	root         Expr
	replacements []Replacement
}

// drawOutcome is a successful draw: the arrivals (swapIn), the un-drained
// pool remainder (leftover, nil when consumed), and the donor rewrites.
type drawOutcome struct {
	swapIn   []Coord
	leftover Expr // nil iff fully consumed
	pending  []pendingReplacement
}

// ReorderLastDim rewrites the last dim so the family pieces form the
// flattened tail. Applicable when the family's total occupancy already
// covers the window (R' >= window) but is misplaced.
func ReorderLastDim(
	env Env,
	layout *Layout,
	family string,
	an *LastDimAnalysis,
	window int,
) error {
	combined := assembleLastDim(env, layout, an.Rest, an.Relevant)
	newValue, err := tentativeRemap(env, layout, combined)
	if err != nil {
		return err
	}
	if err := verifySuffix(newValue, family, window); err != nil {
		return err // nothing committed: layout untouched
	}
	commitLastDim(layout, newValue)
	return nil
}

// DrawIntoLastDim raises the family suffix occupancy to >= window via the
// divisor-search draw: achievable occupancies are R'*draw with draw | E, so
// each feasible draw >= ceil(window/R') is tried in ascending order until
// the donor GCD chain realizes one.
func DrawIntoLastDim(
	env Env,
	layout *Layout,
	family string,
	an *LastDimAnalysis,
	window int,
	totalFreedom int, // V, the family's total freedom
) error {
	vExt := totalFreedom / an.RelevantSize
	minDraw := (window + an.RelevantSize - 1) / an.RelevantSize // ceil(window / R')

	tried := []int{}
	for draw := minDraw; draw <= an.RestSize; draw++ {
		if an.RestSize%draw != 0 || draw > vExt {
			continue
		}
		tried = append(tried, draw)
		outcome := attemptDraw(env, layout, family, an.Rest, an.RestSize, draw)
		if outcome == nil {
			continue
		}

		nonLane := []Coord{}
		if outcome.leftover != nil {
			nonLane = append(nonLane, outcome.leftover.(Coord))
		}
		lane := append([]Coord{}, outcome.swapIn...)
		lane = append(lane, an.Relevant...)
		combined := assembleLastDim(env, layout, nonLane, lane)

		// verify the rebuilt last dim BEFORE committing the donor rewrites,
		// so a nested factorization whose cut structure splits the family
		// block (suffix < window) bails with the layout untouched
		newValue, err := tentativeRemap(env, layout, combined)
		if err != nil {
			return err
		}
		if err := verifySuffix(newValue, family, window); err != nil {
			return err
		}

		// all splits succeeded and verified: commit donors, then the last dim
		for _, p := range outcome.pending {
			p.htuple.SetLeaf(p.idx, Replace(p.root, p.replacements))
		}
		commitLastDim(layout, newValue)
		return nil
	}

	if len(tried) == 0 {
		return fmt.Errorf(
			"window %d is not representable in the last dimension: no draw divisor of "+
				"non-relevant occupancy %d within freedom %d (achievable occupancies are "+
				"multiples of %d dividing %d)",
			window, an.RestSize, vExt, an.RelevantSize, an.RelevantSize*an.RestSize)
	}
	return fmt.Errorf(
		"relevant freedom is present but not extractable: no donor chain realizes "+
			"draw %v (tried in ascending order)",
		tried)
}

// tentativeRemap runs the last-dim remap WITHOUT committing: the access map
// is pure, so the rebuilt coordinate value is returned for verification
// before any mutation lands.
//
// Digit-order convention: the access map assigns a variable's LOW digits to
// the FIRST (most significant) dimensions — the day-one convention
// (dim0 = warp % 7). For a flat last dim the remap is an identity, so piece
// order IS weight order and the family pieces go LAST in the assembly. For
// a nested last dim the sub-access redistributes the combined variable: the
// flattened tail receives the combined variable's FIRST piece, so the
// family pieces go FIRST. CAVEAT: the sub-dimension cut boundaries can
// split the family block (see verifySuffix) — the assembly order alone does
// not guarantee the suffix, hence the explicit verification.
func tentativeRemap(env Env, layout *Layout, combined Coord) (*HTupleValue, error) {
	lastShape := layout.Shape.Last()
	if lastShape.IsRecursive() {
		h, err := AccessWithCoord(env, []Coord{combined}, lastShape.AsHTuple())
		if err != nil {
			return nil, err
		}
		return NewHTupleValueHTuple(h), nil
	}
	flat, err := AccessWithCoord(
		env,
		[]Coord{combined},
		NewConstHTuple([]Value{lastShape.Rank()}),
	)
	if err != nil {
		return nil, err
	}
	check.Check(flat.Size() == 1)
	return NewHTupleValueLeaf(flat.Index(0).Leaf()), nil
}

// verifySuffix checks that the tentatively rebuilt last dim really has the
// family at its flattened tail. For nested last dims the sub-access cut
// boundaries can split the family block — the assembly order alone does not
// guarantee the suffix, so it is verified explicitly.
func verifySuffix(newValue *HTupleValue, family string, window int) error {
	pieces := []Coord{}
	if err := lastDimPieces(newValue, &pieces); err != nil {
		return err
	}
	an, err := classifyPieces(pieces, family)
	if err != nil {
		return err
	}
	if an.Suffix < window {
		return fmt.Errorf(
			"window %d is not representable in the last dimension's factorization: "+
				"the sub-dimension cut structure splits the family block (max suffix %d)",
			window, an.Suffix)
	}
	return nil
}

// commitLastDim installs a verified rebuilt last-dim coordinate value.
func commitLastDim(layout *Layout, newValue *HTupleValue) {
	lastIdx := layout.Coordinate.Size() - 1
	if newValue.IsHTuple() {
		layout.Coordinate.SetHTuple(lastIdx, *newValue.HTuple())
	} else {
		layout.Coordinate.SetLeaf(lastIdx, newValue.Leaf())
	}
}

// attemptDraw tries to draw exactly `draw` freedom from the family pieces
// living in the non-last dimensions (recursing into nested ones), splitting
// the non-relevant pool `rest` (bound E) to compensate. Returns nil when the
// GCD chain over the donors cannot realize this draw.
func attemptDraw(
	env Env,
	layout *Layout,
	family string,
	rest []Coord,
	existedSize int,
	draw int,
) *drawOutcome {
	pool := env.Factory.StrideCoord(rest)
	poolSize := existedSize
	AnalyzeRange(pool)

	out := &drawOutcome{swapIn: []Coord{}}
	remaining := draw

	// walk the non-last dimensions, recursing into nested values
	var donate func(h *HTuple)
	donate = func(h *HTuple) {
		for idx := 0; idx < h.Size() && remaining > 1; idx++ {
			v := h.Index(idx)
			switch {
			case v.IsLeaf():
				donateLeaf(env, h, idx, v.Leaf(), family, &pool, &poolSize,
					&remaining, out)
			case v.IsHTuple():
				donate(v.HTuple())
			case v.IsTuple():
				for _, e := range v.Tuple().vec {
					donateLeaf(env, h, idx, e, family, &pool, &poolSize,
						&remaining, out)
				}
			}
		}
	}
	lastIdx := layout.Coordinate.Size() - 1
	for dim := 0; dim < lastIdx && remaining > 1; dim++ {
		v := layout.Coordinate.Index(dim)
		switch {
		case v.IsLeaf():
			donateLeaf(env, &layout.Coordinate, dim, v.Leaf(), family, &pool,
				&poolSize, &remaining, out)
		case v.IsHTuple():
			donate(v.HTuple())
		case v.IsTuple():
			for _, e := range v.Tuple().vec {
				donateLeaf(env, &layout.Coordinate, dim, e, family, &pool,
					&poolSize, &remaining, out)
			}
		}
	}

	if remaining != 1 {
		return nil // this draw is not realizable by the donor GCD chain
	}
	if poolSize > 1 {
		out.leftover = pool
	}
	return out
}

// donateLeaf draws what it can from the pure-family pieces of one donor
// leaf, recording the replacement that compensates the donor with pool
// freedom. pool/remaining are threaded by pointer; pool is REPLACED with its
// un-drained remainder as portions are taken.
func donateLeaf(
	env Env,
	htuple *HTuple,
	idx int,
	access Expr,
	family string,
	pool *Expr,
	poolSize *int,
	remaining *int,
	out *drawOutcome,
) {
	if !IsCoord(access) {
		return
	}
	dimCoord, err := AnalyzeCoord(access)
	if err != nil {
		return
	}
	if !dimCoord.CoordInfo().HasFamily(family) {
		return
	}
	_, candidativeList := PartiteCoord(dimCoord, family)

	swapInReplacement := []Replacement{}

	for _, cand := range candidativeList {
		if *remaining == 1 {
			break
		}
		ub := int(cand.UpperBound())
		if ub <= 1 {
			continue
		}

		// This # of freedom we need to draw from the cand (ie candidative variable)
		portion := common.GCD(ub, *remaining)
		if portion == 1 {
			continue // shares no factor with the remaining need, skip it
		}

		// (1) Split the candidative freedom with needed portion to be ready to swap in
		AnalyzeRange(cand.(Expr))
		lhs, rhs, err := env.Factory.Split(cand.(Expr), ub/portion, portion)
		if err != nil {
			return
		}

		// (2) Record the swapped in portion, for later swap in
		out.swapIn = append(out.swapIn, rhs.(Coord))

		// (3) Split the swap out variable to obtain portion for replacement
		//
		// - 1) The old expression can be used up, ie lhs == nil
		// - 2) Otherwise, compose a combined variable, ie lhs != nil
		AnalyzeRange(*pool)
		x, replacePortion, err := env.Factory.Split(*pool, *poolSize/portion, portion)
		if err != nil {
			return
		}
		*pool = x // update the pool with its un-drained remainder
		*poolSize /= portion
		*remaining /= portion

		// (4) Figure out the swap in variable
		var rep Expr // replacement expression
		if lhs != nil {
			rep = env.Factory.StrideCoord([]Coord{lhs.(Coord), replacePortion.(Coord)})
		} else {
			rep = replacePortion
		}

		// (5) Record replacement
		swapInReplacement = append(swapInReplacement, Replacement{
			Old: cand.(Expr),
			New: rep,
		})
	}

	if len(swapInReplacement) > 0 {
		out.pending = append(out.pending, pendingReplacement{
			htuple:       htuple,
			idx:          idx,
			root:         access,
			replacements: swapInReplacement,
		})
	}
}

// assembleLastDim orders the new last-dim pieces so the family pieces land
// on the flattened tail after the remap: last for a flat last dim, first for
// a nested one (see the digit-order convention in tentativeRemap).
func assembleLastDim(
	env Env,
	layout *Layout,
	nonLane []Coord,
	lane []Coord,
) Coord {
	var pieces []Coord
	if layout.Shape.Last().IsRecursive() {
		pieces = append(pieces, lane...) // nested: first piece reaches the tail
		pieces = append(pieces, nonLane...)
	} else {
		pieces = append(pieces, nonLane...)
		pieces = append(pieces, lane...) // flat: last pieces are the tail
	}
	out := env.Factory.StrideCoord(pieces)
	AnalyzeRange(out)
	return out.(Coord)
}
