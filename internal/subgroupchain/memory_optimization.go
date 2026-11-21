package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
	"github.com/dianpeng/layoutalgebra/internal/common"
)

// --------------------------------------------------------------------------------------
//
// Memory-hierarchy optimization framework — the universal backbone
//
// This file holds every general component shared by the memory-hierarchy
// passes (GMEM coalescing, SMEM bank-conflict resolution), in reading order:
//
//   1. Pass / Context / Optimize — the pipeline contract and driver
//   2. Last-dim analysis          — flatten + classify the last dimension
//   3. Warp sweep evaluation      — the objective measurement
//   4. Eval                       — the reference expression interpreter
//   5. Coverage repair engine     — reorder + divisor-search draw
//
// The per-hierarchy instantiations (GMem, Smem) live in gmem_map.go and
// smem.go; they only pick an objective, a byte window and (optionally) a
// transform stage.
//
// --------------------------------------------------------------------------------------

// ======================================================================================
// 1. The pipeline: Pass, Context, Optimize
// ======================================================================================

// --------------------------------------------------------------------------------------
//
// Memory-hierarchy optimization framework
//
// The universal pipeline shared by every pass (GMEM coalescing, SMEM bank
// conflict resolution):
//
//   [1] Window:    W = min(last dim extent, pass byte window / dtype, family
//                  freedom V) — the family coverage target in elements
//   [2] Analysis:  flatten + classify the last dim's pieces by family
//                  (LastDimAnalysis)
//   [3] Fast path: the pass's objective is already satisfied (GMEM: family
//                  suffix covers W; SMEM: warp sweep hits pairwise distinct
//                  banks) => no-op
//   [4] Transform: the pass's mapping-independent repair stage (SMEM
//                  swizzle; GMEM has none) — TryMappingRepair
//   [5] Coverage repair (universal engine): reorder the last dim's pieces,
//                  or draw family freedom from the donor dims via the
//                  divisor-search GCD swap
//   [6] Post-check: re-verify the objective; a repair that cannot satisfy it
//                  is reported (eg SMEM conflicts that only a swizzle can
//                  fix)
//
// Contract rules
//
//   - Conservative: do what you can, return an error when you cannot.
//     Infeasible draws are transactional — the layout is left untouched.
//   - Every transform preserves the bijection: dims keep their extents,
//     variables are consumed exactly once.
//   - TryMappingRepair implementations own their transactionality; the
//     swizzle stage is expected to re-verify the objective internally before
//     reporting handled.
//
// --------------------------------------------------------------------------------------

// Context bundles everything a pass stage may need. It is refreshed (new
// analysis) after each layout mutation.
type Context struct {
	Env       Env
	Pass      Pass
	Layout    *Layout
	BoV       []*Variable
	Lane      *Variable // the family's sweep variable, nil when not unique/absent
	DtypeSize int
	Window    int // element window W = min(last dim, byte window / dtype, V)
	Analysis  *LastDimAnalysis
}

// Sweep measures the warp sweep objective (sectors, banks) of the context's
// lane variable.
func (self *Context) Sweep() (SweepStats, error) {
	if self.Lane == nil {
		return SweepStats{}, fmt.Errorf("no unique family variable to sweep")
	}
	return SweepLane(self.Env, self.Layout, self.BoV, self.Lane, self.DtypeSize)
}

// Pass is the per-hierarchy instantiation of the framework.
type Pass interface {
	// Family is the relevant variable family name.
	Family() string

	// ByteWindow returns the pass's byte window — the coalesce line size
	// (GMEM) or the bank transaction span (SMEM: banks x word width). It
	// must be a multiple of the dtype size.
	ByteWindow(dtypeSize int) (int, error)

	// ObjectiveSatisfied reports whether the layout already meets the
	// objective (stage 3 fast path).
	ObjectiveSatisfied(ctx *Context) bool

	// TryMappingRepair is the pass's mapping-independent repair stage
	// (stage 4, eg the SMEM swizzle). Returns handled=false when
	// unavailable; returning handled=true with a nil error asserts the
	// objective is met. Passes without such a stage return (false, nil).
	TryMappingRepair(ctx *Context) (bool, error)

	// Validate rejects inputs the pass cannot reason about, after the
	// context is built (eg SMEM requires a unique family variable for its
	// warp sweep predicate). Returning nil accepts the input.
	Validate(ctx *Context) error
}

// Optimize runs the universal pipeline over the layout.
func Optimize(
	env Env,
	pass Pass,
	bov []*Variable,
	dtypeSize int,
	layout *Layout,
) error {
	family := pass.Family()
	if family == "" {
		return fmt.Errorf("empty relevant variable family")
	}

	// [1]+[2] window and analysis
	ctx, err := newContext(env, pass, bov, dtypeSize, layout)
	if err != nil {
		return err
	}
	if err := pass.Validate(ctx); err != nil {
		return err
	}

	// [3] fast path
	if pass.ObjectiveSatisfied(ctx) {
		return nil
	}

	// [4] pass-specific transform stage (swizzle)
	handled, err := pass.TryMappingRepair(ctx)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	// [5] universal coverage repair
	checkDimExtent(layout, ctx.Analysis)
	if ctx.Analysis.RelevantSize >= ctx.Window {
		if err := ReorderLastDim(env, layout, family, ctx.Analysis, ctx.Window); err != nil {
			return err
		}
	} else {
		v := FamilyFreedom(layout, family)
		if err := DrawIntoLastDim(env, layout, family, ctx.Analysis, ctx.Window, v); err != nil {
			return err
		}
	}

	// [6] post-check: the coverage repair's construction guarantees the
	// suffix objective (assembleLastDim places the family at the flattened
	// tail; the remap preserves it by the digit-order convention), so this
	// re-verification only guards passes with a stronger predicate (eg SMEM
	// bank distinctness). When the re-analysis itself cannot see through the
	// repaired structure (misaligned mixed digits in a nested remap), the
	// construction guarantee stands.
	ctx, err = newContext(env, pass, bov, dtypeSize, layout)
	if err != nil {
		return nil // construction guarantee; see comment above
	}
	if !pass.ObjectiveSatisfied(ctx) {
		return fmt.Errorf(
			"coverage repair could not satisfy the objective (bijection preserved); " +
				"the pass's transform stage (eg swizzle) is required")
	}
	return nil
}

func newContext(
	env Env,
	pass Pass,
	bov []*Variable,
	dtypeSize int,
	layout *Layout,
) (*Context, error) {
	family := pass.Family()

	byteWindow, err := pass.ByteWindow(dtypeSize)
	if err != nil {
		return nil, err
	}
	lastDimExtent := layout.Shape.Last().Rank()
	v := FamilyFreedom(layout, family)
	window := common.Min3(lastDimExtent, byteWindow/dtypeSize, v)

	analysis, err := AnalyzeLastDim(layout, family)
	if err != nil {
		return nil, err
	}

	// the sweep variable: the family's unique variable in the BoV, if any
	var lane *Variable
	for _, varNode := range bov {
		if varNode.Family() == family {
			if lane != nil {
				lane = nil // not unique: sweeps unavailable
				break
			}
			lane = varNode
		}
	}

	return &Context{
		Env:       env,
		Pass:      pass,
		Layout:    layout,
		BoV:       bov,
		Lane:      lane,
		DtypeSize: dtypeSize,
		Window:    window,
		Analysis:  analysis,
	}, nil
}

// ======================================================================================
// 2. Last-dimension analysis
// ======================================================================================

// --------------------------------------------------------------------------------------
//
// Last-dimension analysis — universal component
//
// Every memory-hierarchy pass reasons about the last dimension: its window
// (low-weight flattened tail) carries the unit strides, so both the GMEM
// coalescing objective and the SMEM bank objective are defined there. The
// analysis flattens the (possibly nested) last dimension into its
// mixed-radix piece sequence and classifies the pieces by variable family.
//
// --------------------------------------------------------------------------------------

// LastDimAnalysis is the classified piece sequence of a layout's last
// dimension.
type LastDimAnalysis struct {
	// Pieces is the flattened piece sequence in flatten order (left = high
	// weights, right = low weights; the tail carries the unit strides).
	Pieces []Coord

	// Rest are the pieces with no involvement of the family; their combined
	// bound is RestSize (E).
	Rest     []Coord
	RestSize int

	// Relevant are the pure-family pieces; their combined bound is
	// RelevantSize (R').
	Relevant     []Coord
	RelevantSize int

	// Suffix is the product of the trailing pure-relevant pieces: the
	// contiguous family coverage at the low-weight tail.
	Suffix int
}

// AnalyzeLastDim flattens and classifies the last dimension by family.
func AnalyzeLastDim(layout *Layout, family string) (*LastDimAnalysis, error) {
	lastIdx := layout.Coordinate.Size() - 1
	pieces := []Coord{}
	if err := lastDimPieces(layout.Coordinate.Index(lastIdx), &pieces); err != nil {
		return nil, err
	}
	return classifyPieces(pieces, family)
}

// FamilyFreedom returns the product of the upper bounds of every DISTINCT
// Variable of the given family anywhere in the layout's coordinate tree,
// recursing into nested dimensions. Notes multiple digits can source the
// same Variable node (the coordinate tree is a DAG over its Variables), so
// each variable is counted once, by identity. An absent family yields the
// empty product, 1.
func FamilyFreedom(layout *Layout, family string) int {
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

// classifyPieces splits the flattened pieces into non-relevant / pure-family
// sides and computes the trailing family suffix.
func classifyPieces(pieces []Coord, family string) (*LastDimAnalysis, error) {
	out := &LastDimAnalysis{Pieces: pieces, RestSize: 1, RelevantSize: 1, Suffix: 1}

	// suffix: product of the trailing pure-family pieces
	idx := len(pieces)
	for idx > 0 {
		info := pieces[idx-1].CoordInfo()
		if !info.IsOnlyOccupiedByFamily(family) {
			break
		}
		idx--
	}
	out.Suffix = 1
	for _, p := range pieces[idx:] {
		out.Suffix *= int(p.UpperBound())
	}

	for _, p := range pieces {
		info := p.CoordInfo()
		has := info.HasFamily(family)
		only := info.IsOnlyOccupiedByFamily(family)
		switch {
		case only:
			out.Relevant = append(out.Relevant, p)
			out.RelevantSize *= int(p.UpperBound())
		case !has:
			out.Rest = append(out.Rest, p)
			out.RestSize *= int(p.UpperBound())
		default:
			return nil, fmt.Errorf(
				"last dimension contains an opaque mixed piece (bound %d) that cannot be split",
				int(p.UpperBound()))
		}
	}
	return out, nil
}

// lastDimPieces flattens the last dimension's coordinate value into its
// piece sequence in flatten order, running coordinate analysis on every leaf
// so each piece's CoordInfo is populated.
func lastDimPieces(v *HTupleValue, out *[]Coord) error {
	switch {
	case v.IsLeaf():
		return expandPieces(v.Leaf(), out)
	case v.IsHTuple():
		h := v.HTuple()
		for idx := 0; idx < h.Size(); idx++ {
			if err := lastDimPieces(h.Index(idx), out); err != nil {
				return err
			}
		}
		return nil
	case v.IsTuple():
		for _, e := range v.Tuple().vec {
			if err := expandPieces(e, out); err != nil {
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
//   - a Coordinate sourcing a Stride is a DIGIT of that Stride: in mixed
//     radix, (src % radix) / stride extracts exactly the contiguous run of
//     source pieces whose weights lie in [stride, radix), so it expands into
//     that run — this is what the nested-dim sub-access produces
//   - a Coordinate sourcing a Coordinate flattens algebraically:
//     ((v % rc)/sc % r)/s == (v % min(r*sc, rc)) / (s*sc)
func expandPieces(e Expr, out *[]Coord) error {
	if !IsCoord(e) {
		return fmt.Errorf("last dimension contains a non-coord expression %T", e)
	}
	if _, err := AnalyzeCoord(e); err != nil {
		return err
	}

	switch n := e.(type) {
	case *Stride:
		for _, c := range n.Coord() {
			if err := expandPieces(c, out); err != nil {
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
				if err := expandPieces(piece, out); err != nil {
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

// checkDimExtent asserts the classified pieces cover the last dim's extent —
// a sanity invariant of the disjoint partition.
func checkDimExtent(layout *Layout, an *LastDimAnalysis) {
	rank := layout.Shape.Last().Rank()
	check.Check(
		an.RestSize*an.RelevantSize == rank,
		fmt.Sprintf("classified pieces %d x %d != last dim rank %d",
			an.RestSize, an.RelevantSize, rank),
	)
}

// ======================================================================================
// 3. Warp sweep evaluation
// ======================================================================================

// --------------------------------------------------------------------------------------
//
// Warp sweep evaluation — the universal objective measurement
//
// Every memory-hierarchy objective (GMEM sector count, SMEM bank
// distinctness) is measured the same way: sweep the relevant (lane) variable
// over [0, bound) with every other concurrency variable pinned at 0 and
// reduce the byte offsets to a predicate. This is exactly the per-warp
// contract: a warp's threads differ only in the lane variable.
//
// --------------------------------------------------------------------------------------

const (
	// GmemSectorBytes is the GMEM sector granularity (NVIDIA: 32B).
	GmemSectorBytes = 32
	// SmemBankCount is the number of shared-memory banks per transaction
	// (NVIDIA: 32).
	SmemBankCount = 32
	// SmemBankWidthBytes is the bank word width (NVIDIA: 4B).
	SmemBankWidthBytes = 4
)

// SweepStats reports a warp sweep's byte-offset footprint.
type SweepStats struct {
	// Lo/Hi/Count describe the element-offset set: Count distinct elements
	// spanning [Lo, Hi]. Count == Hi-Lo+1 iff the sweep is contiguous.
	Lo, Hi, Count int

	// Sectors is the number of distinct GmemSectorBytes sectors touched —
	// the GMEM objective (lower is better, contiguous = minimal).
	Sectors int

	// DistinctBanks is the number of distinct SMEM banks touched — the SMEM
	// objective (== min(Count, 32) iff conflict-free).
	DistinctBanks int
}

// Contiguous reports whether the sweep covers a contiguous element range.
func (self *SweepStats) Contiguous() bool {
	return self.Count == self.Hi-self.Lo+1
}

// BankConflictFree reports whether a warp's sweep hits pairwise distinct
// banks: DistinctBanks saturates at min(element count, bank count).
func (self *SweepStats) BankConflictFree() bool {
	maxBanks := self.Count
	if maxBanks > SmemBankCount {
		maxBanks = SmemBankCount
	}
	return self.DistinctBanks == maxBanks
}

// SweepLane evaluates the layout's byte-offset expression over the lane
// variable's full range with every other BoV variable pinned at 0.
func SweepLane(
	env Env,
	layout *Layout,
	bov []*Variable,
	lane *Variable,
	dtypeSize int,
) (SweepStats, error) {
	offset, err := StrideMap(env, layout, dtypeSize)
	if err != nil {
		return SweepStats{}, err
	}

	assignment := make(map[string]int, len(bov))
	for _, v := range bov {
		assignment[v.Name()] = 0
	}

	var out SweepStats
	seenElem := make(map[int]bool)
	seenSector := make(map[int]bool)
	seenBank := make(map[int]bool)

	first := true
	for l := 0; l < int(lane.UpperBound()); l++ {
		assignment[lane.Name()] = l
		byteOff, err := Eval(offset, assignment)
		if err != nil {
			return SweepStats{}, err
		}
		if byteOff < 0 {
			return SweepStats{}, fmt.Errorf("negative byte offset %d", byteOff)
		}
		elem := byteOff / dtypeSize
		seenElem[elem] = true
		seenSector[byteOff/GmemSectorBytes] = true
		seenBank[(byteOff/SmemBankWidthBytes)%SmemBankCount] = true
		if first || elem < out.Lo {
			out.Lo = elem
		}
		if first || elem > out.Hi {
			out.Hi = elem
		}
		first = false
	}
	out.Count = len(seenElem)
	out.Sectors = len(seenSector)
	out.DistinctBanks = len(seenBank)
	return out, nil
}

// ======================================================================================
// 4. Reference expression interpreter
// ======================================================================================

// Eval is the reference interpreter for the subgroupchain expression IR: it
// concretely assigns every external variable and evaluates the expression
// bottom-up. It is what the end consumer effectively does when generating
// code from a mapped offset expression, and it backs the framework's sweep
// predicates (sector counts, bank distinctness).
//
// Missing variable assignments, division/modulo by zero are errors.
func Eval(e Expr, assignment map[string]int) (int, error) {
	switch n := e.(type) {
	case *Const:
		return n.Value(), nil

	case *Variable:
		v, ok := assignment[n.Name()]
		if !ok {
			return 0, fmt.Errorf("variable %q has no assignment", n.Name())
		}
		return v, nil

	case *Symbol:
		v, ok := assignment[n.Name()]
		if !ok {
			return 0, fmt.Errorf("symbol %q has no assignment", n.Name())
		}
		return v, nil

	case *Binary:
		l, err := Eval(n.L(), assignment)
		if err != nil {
			return 0, err
		}
		r, err := Eval(n.R(), assignment)
		if err != nil {
			return 0, err
		}
		switch n.Op() {
		case Add:
			return l + r, nil
		case Sub:
			return l - r, nil
		case Mul:
			return l * r, nil
		case Div:
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return l / r, nil
		case Mod:
			if r == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			return l % r, nil
		default:
			return 0, fmt.Errorf("unknown binary op %d", n.Op())
		}

	case *Coordinate:
		// radix based digit extraction: (source % radix) / stride
		src, err := Eval(n.Source(), assignment)
		if err != nil {
			return 0, err
		}
		return (src % int(n.Radix())) / int(n.Stride()), nil

	case *Stride:
		total := 0
		stride := n.Stride()
		for i, c := range n.Coord() {
			v, err := Eval(c, assignment)
			if err != nil {
				return 0, err
			}
			total += v * stride[i]
		}
		return total, nil

	default:
		return 0, fmt.Errorf("unknown expression node kind %d", e.Kind())
	}
}

// ======================================================================================
// 5. Coverage repair engine
// ======================================================================================

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
