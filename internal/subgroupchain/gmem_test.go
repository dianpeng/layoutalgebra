package subgroupchain

import (
	"testing"

	"github.com/dianpeng/layoutalgebra/internal/common"
)

// --------------------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------------------

// elementOffsets enumerates the whole BoV assignment space and evaluates the
// layout's element (not byte) offset for each assignment.
func elementOffsets(t *testing.T, env Env, layout *Layout, bov []*Variable, dtypeSize int) map[int][]int {
	t.Helper()
	offset, err := StrideMap(env, layout, dtypeSize)
	if err != nil {
		t.Fatalf("StrideMap failed: %v", err)
	}
	names := common.Map(bov, func(v *Variable) string { return v.Name() })
	bounds := common.Map(bov, func(v *Variable) int { return int(v.UpperBound()) })

	out := make(map[int][]int, common.Product(bounds...))
	forEachAssignment(bounds, func(assignment []int) bool {
		envm := make(map[string]int, len(bov))
		for i := range bov {
			envm[names[i]] = assignment[i]
		}
		elem := evalExpr(t, offset, envm) / dtypeSize
		out[elem] = append(out[elem], 0)
		return true
	})
	return out
}

// assertBijective fails the test unless the layout maps the BoV onto [0, rank)
// one-to-one.
func assertBijective(t *testing.T, env Env, layout *Layout, bov []*Variable, dtypeSize int) {
	t.Helper()
	seen := elementOffsets(t, env, layout, bov, dtypeSize)
	rank := layout.Shape.Rank()
	if len(seen) != rank {
		t.Fatalf("layout maps BoV onto %d distinct elements, want %d (rank)", len(seen), rank)
	}
	for e := range seen {
		if e < 0 || e >= rank {
			t.Fatalf("element offset %d out of range [0,%d)", e, rank)
		}
	}
}

// sectorCount sweeps the lane variable (all other variables pinned at 0) and
// counts the distinct 32-byte GMEM sectors the byte offsets fall into.
func sectorCount(t *testing.T, env Env, layout *Layout, bov []*Variable, lane *Variable, dtypeSize int) int {
	t.Helper()
	offset, err := StrideMap(env, layout, dtypeSize)
	if err != nil {
		t.Fatalf("StrideMap failed: %v", err)
	}
	assignment := make(map[string]int, len(bov))
	for _, v := range bov {
		assignment[v.Name()] = 0
	}
	sectors := make(map[int]bool)
	for l := 0; l < int(lane.UpperBound()); l++ {
		assignment[lane.Name()] = l
		sectors[evalExpr(t, offset, assignment)/32] = true
	}
	return len(sectors)
}

// laneOccupancy re-analyzes the last dimension and returns the product of the
// bounds of its pure-lane-family pieces (R').
func laneOccupancy(t *testing.T, layout *Layout, family string) int {
	t.Helper()
	last := layout.Coordinate.Index(layout.Coordinate.Size() - 1)
	if !last.IsLeaf() {
		return -1
	}
	va, err := AnalyzeCoord(last.Leaf())
	if err != nil {
		t.Fatalf("AnalyzeCoord failed: %v", err)
	}
	_, relevant := PartiteCoord(va, family)
	return common.Product(coordBounds(relevant)...)
}

// --------------------------------------------------------------------------------------
// scenarios
// --------------------------------------------------------------------------------------

// Full swap: the last dim's non-relevant freedom (warp) is entirely displaced
// by lane freedom drawn from dim 0. Sectors per warp sweep drop 8 -> 4.
func TestGMemFullSwap(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 32)
	warp := f.Variable("warp", "warp", 8)
	tile := NewConstHTuple([]Value{8, 32})

	coord, err := Access(env, []*Variable{lane, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}

	if got := laneOccupancy(t, layout, "lane"); got != 4 {
		t.Fatalf("pre-GMem lane occupancy = %d, want 4", got)
	}
	// dim1 = Stride([lane-high(4), warp(8)]) interleaves the lane digits with
	// the warp digits, so each lane lands in its own 32B sector: 32 sectors
	before := sectorCount(t, env, layout, []*Variable{lane, warp}, lane, 4)
	if before != 32 {
		t.Fatalf("pre-GMem sectors = %d, want 32", before)
	}

	if err := GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatalf("GMem failed: %v", err)
	}

	assertBijective(t, env, layout, []*Variable{lane, warp}, 4)
	if got := laneOccupancy(t, layout, "lane"); got != 32 {
		t.Errorf("post-GMem lane occupancy = %d, want 32 (full window)", got)
	}
	after := sectorCount(t, env, layout, []*Variable{lane, warp}, lane, 4)
	if after != 4 {
		t.Errorf("post-GMem sectors = %d, want 4 (128B contiguous per warp)", after)
	}
}

// Fast path: the last dim is already purely lane — GMem must be a no-op.
func TestGMemAlreadyOptimalNoOp(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	warp := f.Variable("warp", "warp", 8)
	lane := f.Variable("lane", "lane", 32)
	tile := NewConstHTuple([]Value{8, 32})

	coord, err := Access(env, []*Variable{warp, lane}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}

	before := elementOffsets(t, env, layout, []*Variable{warp, lane}, 4)
	if err := GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatalf("GMem failed: %v", err)
	}
	after := elementOffsets(t, env, layout, []*Variable{warp, lane}, 4)

	if len(before) != len(after) {
		t.Fatalf("no-op GMem changed the element count: %d -> %d", len(before), len(after))
	}
	for e := range before {
		if _, ok := after[e]; !ok {
			t.Fatalf("no-op GMem dropped element %d", e)
		}
	}
	if got := sectorCount(t, env, layout, []*Variable{warp, lane}, lane, 4); got != 4 {
		t.Errorf("sectors = %d, want 4", got)
	}
}

// Partial window: the line (32B = 8 elements) is smaller than the last dim
// (32), so only 2 lane freedom is drawn in; the warp pool remainder stays.
func TestGMemPartialWindow(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 32)
	warp := f.Variable("warp", "warp", 8)
	tile := NewConstHTuple([]Value{8, 32})

	coord, err := Access(env, []*Variable{lane, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}

	if err := GMem(env, GMemCoalesce{ByteSize: 32, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatalf("GMem failed: %v", err)
	}

	assertBijective(t, env, layout, []*Variable{lane, warp}, 4)
	// window = min(32, 32/4=8, V=32) = 8: occupancy must be exactly 8
	if got := laneOccupancy(t, layout, "lane"); got != 8 {
		t.Errorf("post-GMem lane occupancy = %d, want 8 (window)", got)
	}
}

// GMem is idempotent: a second run finds the fast path and changes nothing.
func TestGMemIdempotent(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 32)
	warp := f.Variable("warp", "warp", 8)
	tile := NewConstHTuple([]Value{8, 32})

	coord, _ := Access(env, []*Variable{lane, warp}, tile)
	layout := &Layout{Coordinate: coord, Shape: tile}

	if err := GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatalf("first GMem failed: %v", err)
	}
	once := elementOffsets(t, env, layout, []*Variable{lane, warp}, 4)

	if err := GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatalf("second GMem failed: %v", err)
	}
	twice := elementOffsets(t, env, layout, []*Variable{lane, warp}, 4)

	if len(once) != len(twice) {
		t.Fatalf("second GMem changed the mapping: %d -> %d elements", len(once), len(twice))
	}
	for e := range once {
		if _, ok := twice[e]; !ok {
			t.Fatalf("second GMem dropped element %d", e)
		}
	}
}

// --------------------------------------------------------------------------------------
// error paths
// --------------------------------------------------------------------------------------

// Infeasible draws must fail loudly AND leave the layout untouched.
func TestGMemErrorsLeaveLayoutUnchanged(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	cases := []struct {
		name   string
		lane   int
		other  int
		shape  []Value
		config GMemCoalesce
	}{
		{
			// window 5 vs occupancy 4 (R'=4, E=3): the only draw divisor 3
			// exceeds the external freedom 2 -> window not representable
			name:  "window-not-representable",
			lane:  8,
			other: 3,
			shape: []Value{2, 12},
			config: GMemCoalesce{
				ByteSize:       20, // line = 5 elements
				VariableFamily: "lane",
			},
		},
		{
			// remaining lane freedom elsewhere (bound 3) shares no factor
			// with the draw (2) -> present but not extractable
			name:  "not-extractable",
			lane:  6,
			other: 2,
			shape: []Value{3, 4},
			config: GMemCoalesce{
				ByteSize:       16, // window 4, occupancy 2, draw 2
				VariableFamily: "lane",
			},
		},
	}

	for _, c := range cases {
		lane := f.Variable("lane", "lane", c.lane)
		other := f.Variable("warp", "warp", c.other)
		tile := NewConstHTuple(c.shape)
		coord, err := Access(env, []*Variable{lane, other}, tile)
		if err != nil {
			t.Fatalf("%s: Access failed: %v", c.name, err)
		}
		layout := &Layout{Coordinate: coord, Shape: tile}
		before := elementOffsets(t, env, layout, []*Variable{lane, other}, 4)

		if err := GMem(env, c.config, 4, layout); err == nil {
			t.Errorf("%s: expected error", c.name)
			continue
		}
		// the layout must be untouched: same element set as before
		after := elementOffsets(t, env, layout, []*Variable{lane, other}, 4)
		if len(before) != len(after) {
			t.Fatalf("%s: failed GMem mutated the layout: %d -> %d elements",
				c.name, len(before), len(after))
		}
		for e := range before {
			if _, ok := after[e]; !ok {
				t.Fatalf("%s: failed GMem dropped element %d", c.name, e)
			}
		}
	}
}

// The divisor search must recover windows that are not an exact multiple of
// the current occupancy: R'=3, W=4 -> draw=2 gives occupancy 6 >= 4.
func TestGMemDivisorSearchCoverage(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 6)
	warp := f.Variable("warp", "warp", 2)
	tile := NewConstHTuple([]Value{2, 6})

	coord, err := Access(env, []*Variable{lane, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}

	if err := GMem(env, GMemCoalesce{ByteSize: 16, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatalf("GMem failed: %v", err)
	}

	assertBijective(t, env, layout, []*Variable{lane, warp}, 4)
	// occupancy 6 >= window 4, and the lane suffix is contiguous:
	// 6 elements * 4 bytes = 24B -> 1 sector per warp
	if got := laneOccupancy(t, layout, "lane"); got != 6 {
		t.Errorf("lane occupancy = %d, want 6 (divisor-search overshoot)", got)
	}
	if got := sectorCount(t, env, layout, []*Variable{lane, warp}, lane, 4); got != 1 {
		t.Errorf("sectors = %d, want 1 (contiguous 24B window)", got)
	}
}

func TestGMemConfigErrors(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 32)
	tile := NewConstHTuple([]Value{32})
	coord, _ := Access(env, []*Variable{lane}, tile)
	layout := &Layout{Coordinate: coord, Shape: tile}

	// line size not a multiple of dtype size
	if err := GMem(env, GMemCoalesce{ByteSize: 30, VariableFamily: "lane"}, 4, layout); err == nil {
		t.Error("ByteSize 30 with dtype 4: expected error")
	}
	// line smaller than dtype
	if err := GMem(env, GMemCoalesce{ByteSize: 2, VariableFamily: "lane"}, 4, layout); err == nil {
		t.Error("ByteSize 2 with dtype 4: expected error")
	}
	// empty family
	if err := GMem(env, GMemCoalesce{ByteSize: 128}, 4, layout); err == nil {
		t.Error("empty family: expected error")
	}
	// family that does not exist anywhere: nothing to optimize, no-op is fine
	if err := GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "ghost"}, 4, layout); err != nil {
		t.Errorf("absent family: expected no-op, got %v", err)
	}
}

// A nested last dimension is optimized like a flat one: the window is
// defined on the flattened tail, and the remap redistributes into the nested
// shape with the relevant pieces at the tail.
func TestGMemNestedLastDim(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 6)
	warp := f.Variable("warp", "warp", 4)
	tile := ConstHTuple{vec: []*ConstHTupleValue{
		NewConstHTupleValueLeaf(4),
		NewConstHTupleValueHTuple(NewConstHTuple([]Value{2, 3})),
	}}

	coord, err := Access(env, []*Variable{lane, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}

	// last dim (2,3) is mixed: lane3 ++ warp2, suffix 1, R'=3, W=6
	if err := GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatalf("GMem failed: %v", err)
	}

	assertBijective(t, env, layout, []*Variable{lane, warp}, 4)
	// suffix lane occupancy 6 = whole last dim -> contiguous 24B -> 1 sector
	if got := sectorCount(t, env, layout, []*Variable{lane, warp}, lane, 4); got != 1 {
		t.Errorf("sectors = %d, want 1 (contiguous 24B window)", got)
	}

	// idempotent: a second run finds the suffix covered
	before := elementOffsets(t, env, layout, []*Variable{lane, warp}, 4)
	if err := GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatalf("second GMem failed: %v", err)
	}
	after := elementOffsets(t, env, layout, []*Variable{lane, warp}, 4)
	if len(before) != len(after) {
		t.Fatalf("second GMem changed the mapping")
	}
}

// A nested donor dimension contributes candidates: the lane freedom living
// inside a nested dim is drawable.
func TestGMemNestedDonor(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 6)
	warp := f.Variable("warp", "warp", 4)
	tile := ConstHTuple{vec: []*ConstHTupleValue{
		NewConstHTupleValueHTuple(NewConstHTuple([]Value{2, 2})),
		NewConstHTupleValueLeaf(6),
	}}

	coord, err := Access(env, []*Variable{lane, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}

	if err := GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatalf("GMem failed: %v", err)
	}

	assertBijective(t, env, layout, []*Variable{lane, warp}, 4)
	if got := sectorCount(t, env, layout, []*Variable{lane, warp}, lane, 4); got != 1 {
		t.Errorf("sectors = %d, want 1", got)
	}
}

// sweepStats sweeps the lane variable (others pinned 0) and reports the
// element-offset set's extent and the distinct 32B sector count.
func sweepStats(
	t *testing.T,
	env Env,
	layout *Layout,
	bov []*Variable,
	lane *Variable,
	dtypeSize int,
) (lo, hi, count, sectors int) {
	t.Helper()
	offset, err := StrideMap(env, layout, dtypeSize)
	if err != nil {
		t.Fatalf("StrideMap failed: %v", err)
	}
	assignment := make(map[string]int, len(bov))
	for _, v := range bov {
		assignment[v.Name()] = 0
	}
	seenElem := make(map[int]bool)
	seenSector := make(map[int]bool)
	for l := 0; l < int(lane.UpperBound()); l++ {
		assignment[lane.Name()] = l
		e := evalExpr(t, offset, assignment) / dtypeSize
		seenElem[e] = true
		seenSector[e*dtypeSize/32] = true
	}
	first := true
	for e := range seenElem {
		if first || e < lo {
			lo = e
		}
		if first || e > hi {
			hi = e
		}
		first = false
	}
	return lo, hi, len(seenElem), len(seenSector)
}

// --------------------------------------------------------------------------------------
// property test: GMem preserves the bijection for every feasible input
// --------------------------------------------------------------------------------------

// testSuffixProduct flattens the last dimension (nesting included, digit-run
// decomposition per the documented expansion rules) and returns the product
// of the trailing pure-lane pieces; -1 when the expansion itself rejects the
// structure (the classified-error cases). The independent semantic verifier
// for coverage is the exact-sector assertion on contiguous sweeps.
func testSuffixProduct(t *testing.T, layout *Layout, family string) int {
	t.Helper()
	g := gmemMap{}
	pieces := []Coord{}
	if err := g.lastDimPieces(layout.Coordinate.Index(layout.Coordinate.Size()-1), &pieces); err != nil {
		return -1
	}
	_, _, suffix, err := g.classifyPieces(pieces, family)
	if err != nil {
		return -1
	}
	return suffix
}

func TestGMemSoundnessExhaustive(t *testing.T) {
	const maxN = 48
	const lineElems = 32 // 128B line, dtype 4

	// nestedVariant wraps the first two extents into one sub-dim:
	// (d0, d1, d2, ...) -> ((d0, d1), d2, ...)
	nestedVariant := func(shape []int) (ConstHTuple, bool) {
		if len(shape) < 2 {
			return ConstHTuple{}, false
		}
		vec := []*ConstHTupleValue{
			NewConstHTupleValueHTuple(NewConstHTuple([]Value{Value(shape[0]), Value(shape[1])})),
		}
		for _, d := range shape[2:] {
			vec = append(vec, NewConstHTupleValueLeaf(Value(d)))
		}
		return ConstHTuple{vec: vec}, true
	}

	cases, optimized := 0, 0
	for n := 2; n <= maxN; n++ {
		for _, bov := range unorderedFactorizations(n, 2) {
			for _, tileShape := range orderedFactorizations(n) {
				shapes := []ConstHTuple{NewConstHTuple(intsToValues(tileShape))}
				if nested, ok := nestedVariant(tileShape); ok {
					shapes = append(shapes, nested)
				}

				for _, tile := range shapes {
					cases++
					env := *NewEnv()
					f := env.Factory

					vars := make([]*Variable, 0, len(bov))
					for i, bound := range bov {
						// first var is warp family, the rest lane; a single
						// variable is the lane itself
						family := "warp"
						if i > 0 || len(bov) == 1 {
							family = "lane"
						}
						vars = append(vars, f.Variable(family, varName(i), bound))
					}

					coord, err := Access(env, vars, tile)
					if err != nil {
						t.Fatalf("N=%d bov=%v tile=%v: Access rejected: %v", n, bov, tileShape, err)
					}
					layout := &Layout{Coordinate: coord, Shape: tile}

					err = GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "lane"}, 4, layout)

					// independent window computation: W = min(last, line, V)
					v := 1
					for _, varNode := range vars {
						if varNode.Family() == "lane" {
							v *= int(varNode.UpperBound())
						}
					}
					window := common.Min3(tileShape[len(tileShape)-1], lineElems, v)

					if err != nil {
						// conservative bailout is allowed, but must leave the
						// bijection intact
						assertBijective(t, env, layout, vars, 4)
						continue
					}
					optimized++
					assertBijective(t, env, layout, vars, 4)

					// on success the last dim's lane SUFFIX must cover the
					// window; and when the whole sweep set is contiguous, the
					// sector count must equal the exact byte-range count
					// (no intra-line striding)
					suffix := testSuffixProduct(t, layout, "lane")
					if suffix >= 0 && suffix < window {
						t.Fatalf("N=%d bov=%v tile=%v: lane suffix %d < window %d",
							n, bov, tileShape, suffix, window)
					}
					var lane *Variable
					for _, varNode := range vars {
						if varNode.Family() == "lane" {
							lane = varNode
							break
						}
					}
					lo, hi, count, sectors := sweepStats(t, env, layout, vars, lane, 4)
					if count == hi-lo+1 { // contiguous sweep
						want := ((hi+1)*4+31)/32 - (lo*4)/32
						if sectors != want {
							t.Fatalf("N=%d bov=%v tile=%v: contiguous sweep [%d,%d] "+
								"sectors %d != exact %d",
								n, bov, tileShape, lo, hi, sectors, want)
						}
					}
				}
			}
		}
	}
	t.Logf("GMem property: %d cases, %d optimized, %d conservative bailouts",
		cases, optimized, cases-optimized)
}
