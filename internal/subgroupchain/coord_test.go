package subgroupchain

import (
	"testing"

	"github.com/dianpeng/layoutalgebra/internal/common"
)

func TestCoordinateNodeBasics(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	v := f.Variable("lane", "lane", 32)
	AnalyzeRange(v)

	digits, err := f.Factorize(v, []int{8, 4})
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
	}

	// IsEqual: same bound + same value expression
	d2, _ := f.Factorize(v, []int{8, 4})
	if !digits[0].IsEqual(d2[0]) {
		t.Error("structurally equal coordinates reported unequal")
	}
	if digits[0].IsEqual(digits[1]) {
		t.Error("different coordinates reported equal")
	}

	// evaluation: Coordinate value == underlying expression value
	for x := 0; x < 32; x++ {
		envm := map[string]int{"lane": x}
		if got := evalExpr(t, digits[0], envm); got != x/4 {
			t.Fatalf("x=%d: digit0 = %d, want %d", x, got, x/4)
		}
	}
}

func TestFactorySplit(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	v := f.Variable("lane", "lane", 24)
	AnalyzeRange(v)

	// 24 = 6 * 4
	lhs, rhs, err := f.Split(v, 6, 4)
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

	// trivial high side yields nil lhs
	lhs2, rhs2, err := f.Split(v, 1, 24)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	if lhs2 != nil {
		t.Errorf("lhs = %v, want nil", lhs2)
	}
	if got := evalExpr(t, rhs2, map[string]int{"lane": 7}); got != 7 {
		t.Errorf("rhs(7) = %d, want 7", got)
	}

	// mismatched product is an error
	if _, _, err := f.Split(v, 4, 4); err == nil {
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

	// dim 0 is the whole warp variable, wrapped in a Coordinate node by
	// Factorize (pure warp family; Partial is true for any Coordinate/Stride
	// composite by definition)
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
	a := f.Variable("warp", "a", 4)
	b := f.Variable("lane", "b", 2)

	// Stride([a4, Stride?]) — build mixed: Stride([a4, b2]) with a pure-relevant
	// subtree buried under another Stride: Stride([a4, Stride([b2])]) is the
	// identity for the inner, so use Stride([a4, c]) where c = Stride of b with
	// a second lane var
	c := f.Variable("lane", "c", 2)
	bc := env.Factory.StrideCoord([]Coord{b, c}).(Coord) // pure lane, bound 4
	mixed := env.Factory.StrideCoord([]Coord{a, bc}).(*Stride)
	AnalyzeRange(mixed)
	AnalyzeCoord(mixed)

	rest, relevant := PartiteCoord(mixed, "lane")
	if len(rest) != 1 || rest[0] != Coord(a) {
		t.Errorf("rest = %v, want [a]", rest)
	}
	// pure-relevant subtree must stay whole, not be exploded into leaves
	if len(relevant) != 1 || relevant[0] != bc {
		t.Errorf("relevant = %v, want the whole pure-lane subtree", relevant)
	}
}

func TestReplaceInsideCoordIR(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	a := f.Variable("warp", "a", 4)
	b := f.Variable("lane", "b", 2)
	c := f.Variable("warp", "c", 2)

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
	x1 := f.Variable("warp", "x1", 2)
	x2 := f.Variable("warp", "x2", 2)
	pair := env.Factory.StrideCoord([]Coord{x1, x2}).(*Stride)
	out2 := Replace(pair, []Replacement{{Old: x1, New: c}})
	got2 := out2.(*Stride)
	if got2.Coord()[0] != Expr(c) || got2.Coord()[1] != Expr(x2) {
		t.Fatalf("single replacement wrong: %v", got2.Coord())
	}
}
