package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
	"github.com/dianpeng/layoutalgebra/internal/common"
)

// --------------------------------------------------------------------------------------
//
// GMEM optimization mapping
//
// (1) The reason why we may have a need for GMEM optimization is due to **unorder of BoV**
//     User provides BoV in a invalid order which result in sub-optimal mapping
//
// (2) The GMEM optimization is simple permutation of varaibles, ie swapping in the relevant
//     variables from else where. The **relevant variables** are defined by **family** of
//     variable. Ie the upper integration must provide a configuration to descripe how the
//     GMEM optimization works, 1) # of bytes to be considered as coalesce lines;
//     2) family of variables that are relevant. Notes 1) defines the relevant bit ranges
//     in offset space. Notes this optimization works in coordinate space, not in offset
//     space, unlike F2.
//
// (3) The GMEM optimization pass is able to optimize all cases
//     - 3.1) Relevant range is partially occupied
//     - 3.2) Relevant range is NOT occupied
//     - 3.3) Relevant range is fully occupied, ie optimal
//     Notes "optimize" means **coverage**: the relevant family must occupy the SUFFIX
//     (the low-weight tail) of the last dimension's flattened piece sequence, with
//     combined bound >= W. Coverage as a bare product is not enough — pieces at high
//     weights interleave the warp sweep across sectors. Genuinely infeasible inputs
//     (no representable window) are reported as classified errors.
//
// (4) Notes the GMEM optimization pass assumes the tiling space may be nested
//
//     - 4.1) The provided tile is not necessarily to be a 1D layout, in fact, in most
//            case it is not, because most compiler do tiling. Both the last dimension
//            (partitioned across its flattened pieces) and the donor dimensions
//            (walked recursively) support nesting
//
//     - 4.2) We only consider the last dimension, notes we assume the tile shape is not
//            permutatable, which is also the fact most likely. It means, a shape of
//            (M0, M1, ..., Mk-1), assuming to be flatten, the Mk-1 dimension is the one
//            we will consider. Assuming the byte range is Z, provided by configuration,
//            and the dtype size is D, we only consider min(Mk-1*D, Z) to be covered by
//            relevant variables. Other part will be ignored totally. Notes, in fact, the
//            dtype will be devided by both sides, ie will really be min(Mk-1, Z/D).
//            We assert on D | Z.
//
//     - 4.3) The window is additionally capped by the total freedom V of the relevant
//            family across the whole mapping: the last dimension can never be covered
//            by more relevant freedom than exists. Hence the optimization window is
//
//                W = min(Mk-1, Z/D, V)
//
//            and the pass tries to raise the relevant SUFFIX occupancy of the last
//            dimension to >= W by drawing freedom from the relevant variables living
//            in the other dimensions, displacing an equal amount of non-relevant
//            freedom back into those dimensions (swap out / swap in).
//
// --------------------------------------------------------------------------------------

type GMemCoalesce struct {
	ByteSize       int    // # of bytes per coalesce line
	VariableFamily string // relevant variable family name
}

type gmemMap struct{}

// windowSize computes W = min(lastDimExtent, lineSize, V) where V is the total
// freedom of the relevant family across the whole layout.
func (self *gmemMap) windowSize(
	dtypeSize int,
	config *GMemCoalesce,
	layout *Layout,
) (int, error) {
	if config.ByteSize < dtypeSize || config.ByteSize%dtypeSize != 0 {
		return 0, fmt.Errorf(
			"coalesce line size %d is not a multiple of dtype size %d",
			config.ByteSize, dtypeSize)
	}
	lineSize := config.ByteSize / dtypeSize
	lastDimExtent := layout.Shape.Last().Rank()
	freedom := self.familyFreedom(layout, config.VariableFamily)
	return common.Min3(lastDimExtent, lineSize, freedom), nil
}

// familyFreedom returns the product of the upper bounds of every DISTINCT
// Variable of the given family anywhere in the layout's coordinate tree,
// recursing into nested dimensions. Notes multiple digits can source the
// same Variable node (the coordinate tree is a DAG over its Variables), so
// each variable is counted once, by identity. An absent family yields the
// empty product, 1.
func (self *gmemMap) familyFreedom(layout *Layout, family string) int {
	out := 1
	seen := make(map[*Variable]bool)
	count := func(e Expr) {
		Visit(
			e,
			func(Expr) bool { return true },
			func(e Expr) error {
				if varNode, ok := e.(*Variable); ok && varNode.Family() == family &&
					!seen[varNode] {
					seen[varNode] = true
					out *= int(varNode.UpperBound())
				}
				return nil
			},
		)
	}
	var walk func(h *HTuple)
	walk = func(h *HTuple) {
		for idx := 0; idx < h.Size(); idx++ {
			v := h.Index(idx)
			switch {
			case v.IsLeaf():
				count(v.Leaf())
			case v.IsHTuple():
				walk(v.HTuple())
			case v.IsTuple():
				for _, e := range v.Tuple().vec {
					count(e)
				}
			}
		}
	}
	walk(&layout.Coordinate)
	return out
}

// --------------------------------------------------------------------------------------
// Last dimension flattening
//
// The last dimension may be nested; its window semantics are defined on the
// FLATTENED piece sequence (left to right = high to low mixed-radix weights),
// because the flattened tail carries the unit strides.
// --------------------------------------------------------------------------------------

// lastDimPieces flattens the last dimension's coordinate value into its
// piece sequence in flatten order, running coordinate analysis on every leaf
// so each piece's CoordInfo is populated.
func (self *gmemMap) lastDimPieces(v *HTupleValue, out *[]Coord) error {
	switch {
	case v.IsLeaf():
		return self.expandPieces(v.Leaf(), out)
	case v.IsHTuple():
		h := v.HTuple()
		for idx := 0; idx < h.Size(); idx++ {
			if err := self.lastDimPieces(h.Index(idx), out); err != nil {
				return err
			}
		}
		return nil
	case v.IsTuple():
		for _, e := range v.Tuple().vec {
			if err := self.expandPieces(e, out); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported last dimension structure")
	}
}

// expandPieces flattens a coord expression into its mixed-radix piece
// sequence:
//
//   - a Stride expands into its operand pieces, recursively
//   - a Coordinate sourcing a Variable is an opaque (pure) leaf piece
//   - a Coordinate sourcing a Stride is a DIGET of that Stride: in mixed
//     radix, (src % radix) / stride extracts exactly the contiguous run of
//     source pieces whose weights lie in [stride, radix), so it expands into
//     that run — this is what the nested-dim sub-access produces
//   - a Coordinate sourcing a Coordinate flattens algebraically:
//     ((v % rc)/sc % r)/s == (v % min(r*sc, rc)) / (s*sc)
func (self *gmemMap) expandPieces(e Expr, out *[]Coord) error {
	if !IsCoord(e) {
		return fmt.Errorf("last dimension contains a non-coord expression %T", e)
	}
	if _, err := AnalyzeCoord(e); err != nil {
		return err
	}

	switch n := e.(type) {
	case *Stride:
		for _, c := range n.Coord() {
			if err := self.expandPieces(c, out); err != nil {
				return err
			}
		}
		return nil

	case *Coordinate:
		// flatten chained Coordinates into an effective (radix, stride) over
		// the ultimate source
		radix, stride := n.Radix(), n.Stride()
		src := n.Source()
		for {
			inner, ok := src.(*Coordinate)
			if !ok {
				break
			}
			innerRadix, innerStride := inner.Radix(), inner.Stride()
			if radix*innerStride > innerRadix {
				radix = innerRadix
			} else {
				radix = radix * innerStride
			}
			stride *= innerStride
			src = inner.Source()
		}

		switch source := src.(type) {
		case *Variable:
			*out = append(*out, n)
			return nil

		case *Stride:
			// decompose the digit into its piece run: pieces whose weights
			// (suffix products of later bounds) lie in [stride, radix). The
			// decomposition is only possible when the digit's (radix, stride)
			// is piece-ALIGNED (both are suffix products of the piece
			// bounds); a misaligned digit crosses piece boundaries and stays
			// an atomic piece — its own analysis still provides the family
			// and bound needed by the suffix check and the reorder.
			weight := 1
			bounds := make([]int, len(source.Coord()))
			for i := len(source.Coord()) - 1; i >= 0; i-- {
				c := source.Coord()[i].(Coord)
				bounds[i] = int(c.UpperBound())
			}
			run := []Expr{}
			runProduct := 1
			for i := len(bounds) - 1; i >= 0; i-- {
				if stride <= weight && weight < radix {
					run = append([]Expr{source.Coord()[i]}, run...)
					runProduct *= bounds[i]
				}
				weight *= bounds[i]
			}
			if runProduct != int(radix/stride) {
				// misaligned digit: atomic piece
				*out = append(*out, n)
				return nil
			}
			for _, piece := range run {
				if err := self.expandPieces(piece, out); err != nil {
					return err
				}
			}
			return nil

		default:
			return fmt.Errorf(
				"last dimension contains an unsupported coordinate source %T", src)
		}

	default:
		return fmt.Errorf("last dimension contains an unsupported coord %T", e)
	}
}

// classifyPieces splits the last dim's flattened pieces into non-relevant
// (rest) and pure-relevant (relevant) sides. A piece that mixes the family
// with others but cannot be expanded (an opaque digit) is rejected — the
// canonical constructions never produce one.
func (self *gmemMap) classifyPieces(
	pieces []Coord,
	family string,
) (rest []Coord, relevant []Coord, suffix int, err error) {
	// suffix: product of the trailing pure-relevant pieces
	idx := len(pieces)
	for idx > 0 {
		info := pieces[idx-1].CoordInfo()
		if !info.IsOnlyOccupiedByFamily(family) {
			break
		}
		idx--
	}
	suffix = 1
	for _, p := range pieces[idx:] {
		suffix *= int(p.UpperBound())
	}

	for _, p := range pieces {
		info := p.CoordInfo()
		has := info.HasFamily(family)
		only := info.IsOnlyOccupiedByFamily(family)
		switch {
		case only:
			relevant = append(relevant, p)
		case !has:
			rest = append(rest, p)
		default:
			return nil, nil, 0, fmt.Errorf(
				"last dimension contains an opaque mixed piece (bound %d) that cannot be split",
				int(p.UpperBound()))
		}
	}
	return rest, relevant, suffix, nil
}

// remapLastDim re-runs the access map for the last dimension against its
// (possibly nested) shape.
//
// Notes the access map assigns a variable's LOW digits to the FIRST (most
// significant) dimensions — the day-one convention (dim0 = warp % 7). For a
// flat last dim the remap is an identity, so piece order IS weight order and
// the relevant pieces go LAST. For a nested last dim the sub-access
// redistributes the combined variable: the flattened tail receives the
// combined variable's FIRST piece — so the relevant pieces go FIRST. The
// caller builds `combined` accordingly (see assembleLastDim).
func (self *gmemMap) remapLastDim(env *Env, layout *Layout, combined Coord) error {
	lastShape := layout.Shape.Last()
	lastIdx := layout.Coordinate.Size() - 1
	if lastShape.IsRecursive() {
		newHTuple, err := AccessWithCoord(*env, []Coord{combined}, lastShape.AsHTuple())
		if err != nil {
			return err
		}
		layout.Coordinate.SetHTuple(lastIdx, newHTuple)
	} else {
		flat, err := AccessWithCoord(
			*env,
			[]Coord{combined},
			NewConstHTuple([]Value{lastShape.Rank()}),
		)
		if err != nil {
			return err
		}
		check.Check(flat.Size() == 1)
		layout.Coordinate.SetLeaf(lastIdx, flat.Index(0).Leaf())
	}
	return nil
}

// assembleLastDim orders the new last-dim pieces so the relevant (family)
// pieces land on the flattened tail after the remap: last for a flat last
// dim, first for a nested one (see remapLastDim for the digit-order
// convention).
func (self *gmemMap) assembleLastDim(
	env *Env,
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

// --------------------------------------------------------------------------------------
// Swap out / swap in with divisor-search draw
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

// attemptDraw tries to draw exactly `draw` freedom from the relevant pieces
// living in the non-last dimensions (recursing into nested ones), splitting
// the non-relevant pool `rest` (bound E) to compensate. Returns nil when the
// GCD chain over the donors cannot realize this draw.
func (self *gmemMap) attemptDraw(
	env *Env,
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
				self.donateLeaf(env, h, idx, v.Leaf(), family, &pool, &poolSize,
					&remaining, out)
			case v.IsHTuple():
				donate(v.HTuple())
			case v.IsTuple():
				for _, e := range v.Tuple().vec {
					self.donateLeaf(env, h, idx, e, family, &pool, &poolSize,
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
			self.donateLeaf(env, &layout.Coordinate, dim, v.Leaf(), family, &pool,
				&poolSize, &remaining, out)
		case v.IsHTuple():
			donate(v.HTuple())
		case v.IsTuple():
			for _, e := range v.Tuple().vec {
				self.donateLeaf(env, &layout.Coordinate, dim, e, family, &pool,
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

// donateLeaf draws what it can from the pure-relevant pieces of one donor
// leaf, recording the replacement that compensates the donor with pool
// freedom. pool/remaining are threaded by pointer; pool is REPLACED with its
// un-drained remainder as portions are taken.
func (self *gmemMap) donateLeaf(
	env *Env,
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

// adjust raises the last dim's relevant suffix occupancy to >= window via a
// divisor search: achievable occupancies are R'*draw where draw divides the
// non-relevant occupancy E, so the pass tries each feasible draw >= ceil(W/R')
// in ascending order until the donor GCD chain realizes one.
func (self *gmemMap) adjust(
	env *Env,
	layout *Layout,
	family string,
	rest []Coord,
	relevant []Coord,
	existedSize int, // E
	relevantSize int, // R'
	window int,
	totalFreedom int, // V
) error {
	vExt := totalFreedom / relevantSize
	minDraw := (window + relevantSize - 1) / relevantSize // ceil(W / R')

	tried := []int{}
	for draw := minDraw; draw <= existedSize; draw++ {
		if existedSize%draw != 0 || draw > vExt {
			continue
		}
		tried = append(tried, draw)
		outcome := self.attemptDraw(env, layout, family, rest, existedSize, draw)
		if outcome == nil {
			continue
		}

		// all splits succeeded: commit the rewrites into the donor dimensions
		for _, p := range outcome.pending {
			p.htuple.SetLeaf(p.idx, Replace(p.root, p.replacements))
		}

		nonLane := []Coord{}
		if outcome.leftover != nil {
			nonLane = append(nonLane, outcome.leftover.(Coord))
		}
		lane := append([]Coord{}, outcome.swapIn...)
		lane = append(lane, relevant...)
		combined := self.assembleLastDim(env, layout, nonLane, lane)
		return self.remapLastDim(env, layout, combined)
	}

	if len(tried) == 0 {
		return fmt.Errorf(
			"window %d is not representable in the last dimension: no draw divisor of "+
				"non-relevant occupancy %d within freedom %d (achievable occupancies are "+
				"multiples of %d dividing %d)",
			window, existedSize, vExt, relevantSize, relevantSize*existedSize)
	}
	return fmt.Errorf(
		"relevant freedom is present but not extractable: no donor chain realizes "+
			"draw %v (tried in ascending order)",
		tried)
}

func (self *gmemMap) run(
	env *Env,
	config GMemCoalesce,
	dtypeSize int,
	layout *Layout,
) error {
	if config.VariableFamily == "" {
		return fmt.Errorf("empty relevant variable family")
	}
	family := config.VariableFamily

	// [1] get the corresponding maximized optimizable size
	window, err := self.windowSize(dtypeSize, &config, layout)
	if err != nil {
		return err
	}

	// [2] flatten the last dimension (nesting included) into its piece
	// sequence, with coordinate analysis populated
	lastIdx := layout.Coordinate.Size() - 1
	pieces := []Coord{}
	if err := self.lastDimPieces(layout.Coordinate.Index(lastIdx), &pieces); err != nil {
		return err
	}

	rest, relevant, suffix, err := self.classifyPieces(pieces, family)
	if err != nil {
		return err
	}

	// [3] fast path: the relevant suffix already covers the window
	if suffix >= window {
		return nil // already optimal, do nothing
	}

	// [4] if the relevant freedom is all present but misplaced, a pure
	// reorder of the piece sequence fixes the suffix without any draw
	relevantSize := 1
	for _, p := range relevant {
		relevantSize *= int(p.UpperBound())
	}
	existedSize := 1
	for _, p := range rest {
		existedSize *= int(p.UpperBound())
	}
	check.Check(existedSize*relevantSize == layout.Shape.Last().Rank())

	if relevantSize >= window {
		// all the relevant freedom is already in the dim, just misplaced:
		// reorder the pieces (relevant to the tail) and remap, no draw
		combined := self.assembleLastDim(env, layout, rest, relevant)
		return self.remapLastDim(env, layout, combined)
	}

	// [5] readjust: divisor-search draw from the donor dimensions
	totalFreedom := self.familyFreedom(layout, family)
	return self.adjust(
		env, layout, family, rest, relevant,
		existedSize, relevantSize, window, totalFreedom,
	)
}

func GMem(env Env, config GMemCoalesce, dtypeSize int, layout *Layout) error {
	x := gmemMap{}
	return x.run(&env, config, dtypeSize, layout)
}
