package subgroupchain

import (
	"testing"

	"github.com/dianpeng/layoutalgebra/internal/common"
)

func TestCoordinateNodeBasics(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	v := f.Variable("lane", "lane", 32)
	dv := f.Dedicate(v)
	AnalyzeRange(dv)

	if !IsCoord(dv) {
		t.Fatalf("dedicated variable is not a Coord")
	}
	if dv.Kind() != KindCoordinate {
		t.Errorf("dedication kind = %d, want KindCoordinate", dv.Kind())
	}
	if !dv.IsFullDedication() {
		t.Error("Dedicate must produce a full dedication")
	}
	// a raw Variable is NOT a Coord anymore
	if IsCoord(v) {
		t.Error("raw Variable must not be a Coord node")
	}

	digits, err := f.Factorize(dv, []int{8, 4})
	if err != nil {
		t.Fatalf("Factorize failed: %v", err)
	}
	for i, d := range digits {
		if !IsCoord(d) {
			t.Fatalf("digit %d is not a Coord: %T", i, d)
		}
		c := d.(*Coordinate)
		if c.Kind() != KindCoordinate {
			t.Errorf("digit %d kind = %d, want KindCoordinate", i, c.Kind())
		}
		if int(c.UpperBound()) != []int{8, 4}[i] {
			t.Errorf("digit %d upper bound = %d, want %d", i, c.UpperBound(), []int{8, 4}[i])
		}
		// spec consistency: bound == radix / stride
		if c.UpperBound() != c.Radix()/c.Stride() {
			t.Errorf("digit %d: bound %d != radix/stride %d", i, c.UpperBound(), c.Radix()/c.Stride())
		}
	}

	// radix spec: 32 = 8 * 4 — high digit (lane/4) is radix 32 stride 4, low
	// digit (lane%4) is radix 4 stride 1. Notes the digits retarget the raw
	// variable directly (the full dedication is flattened by the factory),
	// so the chain terminates immediately
	if d0 := digits[0].(*Coordinate); d0.Radix() != 32 || d0.Stride() != 4 {
		t.Errorf("digit0 spec = (%d, %d), want (32, 4)", d0.Radix(), d0.Stride())
	}
	if src, ok := digits[0].(*Coordinate).Source().(*Variable); !ok || src.Name() != "lane" {
		t.Errorf("digit0 source = %T, want the lane Variable", digits[0].(*Coordinate).Source())
	}
	if d1 := digits[1].(*Coordinate); d1.Radix() != 4 || d1.Stride() != 1 {
		t.Errorf("digit1 spec = (%d, %d), want (4, 1)", d1.Radix(), d1.Stride())
	}

	// IsEqual: same spec + same source
	d2, _ := f.Factorize(dv, []int{8, 4})
	if !digits[0].IsEqual(d2[0]) {
		t.Error("structurally equal coordinates reported unequal")
	}
	if digits[0].IsEqual(digits[1]) {
		t.Error("different coordinates reported equal")
	}

	// evaluation: (lane % radix) / stride
	for x := 0; x < 32; x++ {
		envm := map[string]int{"lane": x}
		if got := evalExpr(t, digits[0], envm); got != x/4 {
			t.Fatalf("x=%d: digit0 = %d, want %d", x, got, x/4)
		}
		if got := evalExpr(t, digits[1], envm); got != x%4 {
			t.Fatalf("x=%d: digit1 = %d, want %d", x, got, x%4)
		}
	}
}

func TestFactorySplit(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	v := f.Variable("lane", "lane", 24)
	dv := f.Dedicate(v)
	AnalyzeRange(dv)

	// 24 = 6 * 4
	lhs, rhs, err := f.Split(dv, 6, 4)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	for x := 0; x < 24; x++ {
		envm := map[string]int{"lane": x}
		if got := evalExpr(t, lhs, envm); got != x/4 {
			t.Fatalf("x=%d: lhs = %d, want %d", x, got, x/4)
		}
		if got := evalExpr(t, rhs, envm); got != x%4 {
			t.Fatalf("x=%d: rhs = %d, want %d", x, got, x%4)
		}
	}

	// trivial high side yields nil lhs; the low side is a FULL DEDICATION
	// coordinate now (never the raw input node)
	lhs2, rhs2, err := f.Split(dv, 1, 24)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	if lhs2 != nil {
		t.Errorf("lhs = %v, want nil", lhs2)
	}
	if !IsCoord(rhs2) {
		t.Errorf("rhs = %T, want a Coord", rhs2)
	}
	if got := evalExpr(t, rhs2, map[string]int{"lane": 7}); got != 7 {
		t.Errorf("rhs(7) = %d, want 7", got)
	}

	// mismatched product is an error
	if _, _, err := f.Split(dv, 4, 4); err == nil {
		t.Error("Split with mismatched product: expected error")
	}
}

func TestAnalyzeCoordFamilies(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	warp := f.Variable("warp", "warp", 8)
	lane := f.Variable("lane", "lane", 32)
	tile := NewConstHTuple([]Value{8, 32})

	coord, err := Access(env, []*Variable{warp, lane}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}

	// dim 0 is the whole warp variable, as a full-dedication Coordinate
	dim0, err := AnalyzeCoord(coord.Index(0).Leaf())
	if err != nil {
		t.Fatalf("AnalyzeCoord failed: %v", err)
	}
	if !dim0.CoordInfo().IsOnlyOccupiedByFamily("warp") {
		t.Errorf("dim0 families = %v, want only [warp]", dim0.CoordInfo().Family)
	}
	if dim0.CoordInfo().Name[0] != "warp" {
		t.Errorf("dim0 names = %v, want [warp]", dim0.CoordInfo().Name)
	}
	if dim0.CoordInfo().Partial {
		t.Error("full dedication reported Partial")
	}
	c, ok := dim0.(*Coordinate)
	if !ok || !c.IsFullDedication() {
		t.Error("dim0 should be a full-dedication Coordinate")
	}

	// dim 1 should be the lane variable (pure lane family)
	dim1, err := AnalyzeCoord(coord.Index(1).Leaf())
	if err != nil {
		t.Fatalf("AnalyzeCoord failed: %v", err)
	}
	if !dim1.CoordInfo().IsOnlyOccupiedByFamily("lane") {
		t.Errorf("dim1 families = %v, want only [lane]", dim1.CoordInfo().Family)
	}
}

func TestAnalyzeCoordMixedAndPartite(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	// BoV order chosen so the last dim mixes warp and lane freedom:
	// tile (8, 32), bov [lane:32, warp:8]:
	//   dim0 = lane%8, dim1 = Stride([warp8, lane4])
	lane := f.Variable("lane", "lane", 32)
	warp := f.Variable("warp", "warp", 8)
	tile := NewConstHTuple([]Value{8, 32})

	coord, err := Access(env, []*Variable{lane, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}

	last := coord.Index(1).Leaf()
	va, err := AnalyzeCoord(last)
	if err != nil {
		t.Fatalf("AnalyzeCoord failed: %v", err)
	}
	info := va.CoordInfo()
	if info.Partial != true {
		t.Error("mixed Stride dim should be Partial")
	}
	if !common.SliceEqual(info.Family, []string{"lane", "warp"}) {
		t.Errorf("families = %v, want [lane warp]", info.Family)
	}
	if int(va.UpperBound()) != 32 {
		t.Errorf("upper bound = %d, want 32", va.UpperBound())
	}

	// Partite: non-lane (warp8) vs lane (4) pieces, products 8 and 4
	rest, relevant := PartiteCoord(va, "lane")
	if len(rest) != 1 || int(rest[0].UpperBound()) != 8 {
		t.Errorf("rest = %v, want a single bound-8 piece", coordBounds(rest))
	}
	if len(relevant) != 1 || int(relevant[0].UpperBound()) != 4 {
		t.Errorf("relevant = %v, want a single bound-4 piece", coordBounds(relevant))
	}
	// disjoint cover: product of both sides == the dim extent
	if common.Product(coordBounds(rest)...)*common.Product(coordBounds(relevant)...) != 32 {
		t.Error("partite sides do not cover the dimension")
	}
	// the rest side must have no lane involvement
	for _, c := range rest {
		if c.CoordInfo().HasFamily("lane") {
			t.Error("rest side contains lane family")
		}
	}

	// non-coord root is rejected
	if _, err := AnalyzeCoord(f.Const(3)); err == nil {
		t.Error("AnalyzeCoord on non-Coord: expected error")
	}
}

func coordBounds(list []Coord) []int {
	return common.Map(list, func(c Coord) int { return int(c.UpperBound()) })
}

func TestPartiteCoordPureSubtreeKeptWhole(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	a := f.Dedicate(f.Variable("warp", "a", 4))
	b := f.Dedicate(f.Variable("lane", "b", 2))
	c := f.Dedicate(f.Variable("lane", "c", 2))

	// mixed: Stride([a4, Stride([b2, c2])]) with a pure-relevant subtree
	bc := env.Factory.StrideCoord([]Coord{b, c}).(Coord) // pure lane, bound 4
	mixed := env.Factory.StrideCoord([]Coord{a, bc}).(*Stride)
	AnalyzeRange(mixed)
	AnalyzeCoord(mixed)

	rest, relevant := PartiteCoord(mixed, "lane")
	if len(rest) != 1 || rest[0] != a {
		t.Errorf("rest = %v, want [a]", rest)
	}
	// pure-relevant subtree must stay whole, not be exploded into leaves
	if len(relevant) != 1 || relevant[0] != bc {
		t.Errorf("relevant = %v, want the whole pure-lane subtree", relevant)
	}
}

// TestPartiteCoordDedicationOfMixed checks the B6 shape: a Coordinate that
// dedicates a mixed Stride (eg the remapped last dim after GMEM) must be
// partitionable — the chain continues through the source.
func TestPartiteCoordDedicationOfMixed(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	warp := f.Dedicate(f.Variable("warp", "warp", 8))
	lane := f.Dedicate(f.Variable("lane", "lane", 4))

	mixed := env.Factory.StrideCoord([]Coord{warp, lane}).(*Stride) // bound 32
	AnalyzeRange(mixed)

	// full dedication of the mixed pool: radix 32, stride 1
	dedication := f.coordinate(mixed, 32, 1)
	AnalyzeRange(dedication)
	va, err := AnalyzeCoord(dedication)
	if err != nil {
		t.Fatalf("AnalyzeCoord failed: %v", err)
	}
	if len(va.CoordInfo().Family) != 2 {
		t.Fatalf("dedication families = %v, want 2", va.CoordInfo().Family)
	}

	// partition must recurse through the dedication into the pool pieces
	rest, relevant := PartiteCoord(va, "lane")
	if common.Product(coordBounds(rest)...) != 8 {
		t.Errorf("rest product = %d, want 8", common.Product(coordBounds(rest)...))
	}
	if common.Product(coordBounds(relevant)...) != 4 {
		t.Errorf("relevant product = %d, want 4", common.Product(coordBounds(relevant)...))
	}
}

func TestReplaceInsideCoordIR(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	a := f.Dedicate(f.Variable("warp", "a", 4))
	b := f.Dedicate(f.Variable("lane", "b", 2))
	c := f.Dedicate(f.Variable("warp", "c", 2))

	// dim = Stride([a4, Stride([b2, c2])]) — replacement target is b, nested
	inner := env.Factory.StrideCoord([]Coord{b, c}).(*Stride)
	root := env.Factory.StrideCoord([]Coord{a, inner}).(*Stride)

	rep := env.Factory.StrideCoord([]Coord{c, b}).(*Stride)
	out := Replace(root, []Replacement{{Old: b, New: rep}})

	got, ok := out.(*Stride)
	if !ok {
		t.Fatalf("Replace returned %T, want *Stride", out)
	}
	// b was nested inside inner: outer slot still holds inner, whose first
	// coord is now rep
	if got.Coord()[1] != Expr(inner) {
		t.Fatalf("outer slot mutated: %v", got.Coord()[1])
	}
	newInner, ok := got.Coord()[1].(*Stride)
	if !ok {
		t.Fatalf("inner is %T, want *Stride", got.Coord()[1])
	}
	if newInner.Coord()[0] != Expr(rep) {
		t.Fatalf("nested replacement did not fire: %v", newInner.Coord()[0])
	}
	if newInner.Coord()[1] != Expr(c) {
		t.Fatalf("inner sibling mutated: %v", newInner.Coord()[1])
	}
	// the rest is untouched
	if got.Coord()[0] != Expr(a) {
		t.Fatalf("outer coord mutated: %v", got.Coord()[0])
	}

	// replacement fires once per Old node identity
	x1 := f.Dedicate(f.Variable("warp", "x1", 2))
	x2 := f.Dedicate(f.Variable("warp", "x2", 2))
	pair := env.Factory.StrideCoord([]Coord{x1, x2}).(*Stride)
	out2 := Replace(pair, []Replacement{{Old: x1, New: c}})
	got2 := out2.(*Stride)
	if got2.Coord()[0] != Expr(c) || got2.Coord()[1] != Expr(x2) {
		t.Fatalf("single replacement wrong: %v", got2.Coord())
	}
}

// TestCoordChainInvariants verifies the structural IR invariants on real
// layouts (pre and post GMEM):
//   - every coordinate access expression is a Coord (no raw Variable leaves)
//   - Coordinate specs are consistent (stride >= 1, radix % stride == 0,
//     bound == radix/stride) and never extract beyond their source
//   - coord chain bounds are non-increasing (monotonicity)
func TestCoordChainInvariants(t *testing.T) {
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

	// check verifies, for each coord node, that its bound never exceeds the
	// window of the freedom it draws from: a Coordinate's bound <= its
	// source's bound, and a Stride's every operand bound <= its rank. Chasing
	// the coord chain downward, bounds are therefore non-increasing.
	var checkNode func(e Expr)
	checkNode = func(e Expr) {
		switch n := e.(type) {
		case *Variable:
			return // freedom provider, outside the coord chain
		case *Coordinate:
			if !IsCoord(n) {
				t.Errorf("Coordinate is not a Coord")
			}
			if n.Stride() < 1 || n.Radix()%n.Stride() != 0 {
				t.Errorf("inconsistent spec: radix=%d stride=%d", n.Radix(), n.Stride())
			}
			if n.UpperBound() != n.Radix()/n.Stride() {
				t.Errorf("bound %d != radix/stride %d", n.UpperBound(), n.Radix()/n.Stride())
			}
			if src, ok := n.Source().(Coord); ok {
				if int(n.UpperBound()) > int(src.UpperBound()) {
					t.Errorf("chain monotonicity violated: %d > source %d",
						n.UpperBound(), src.UpperBound())
				}
			}
			checkNode(n.Source())
		case *Stride:
			rank := 1
			for _, ch := range n.Coord() {
				cc, ok := ch.(Coord)
				if !ok {
					t.Fatalf("Stride operand is not a Coord: %T", ch)
				}
				if int(cc.UpperBound()) > n.Rank() {
					t.Errorf("stride operand %d exceeds rank %d", cc.UpperBound(), n.Rank())
				}
				rank *= int(cc.UpperBound())
			}
			if rank != n.Rank() {
				t.Errorf("stride rank %d != product of operands %d", n.Rank(), rank)
			}
			for _, ch := range n.Coord() {
				checkNode(ch)
			}
		}
	}

	for i := 0; i < layout.Coordinate.Size(); i++ {
		v := layout.Coordinate.Index(i)
		if !v.IsLeaf() || !IsCoord(v.Leaf()) {
			t.Fatalf("dim %d is not a coordinate access expression", i)
		}
		checkNode(v.Leaf())
	}
}
