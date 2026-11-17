package subgroupchain

import "testing"

// TestAccessReadmeExample is inspired by the README's worked example:
//
//	BoV: (warp, lane) with 0 <= warp < 14, 0 <= lane < 32
//	tile shape [7][16][4]
//
// The access map peels the low digit of each variable per dimension (via GCD),
// which yields the bijective mapping:
//
//	dim 0 (extent 7):  coord = warp % 7
//	dim 1 (extent 16): coord = ((warp / 7) % 2) * 8 + (lane % 8)
//	dim 2 (extent 4):  coord = lane / 8
//
// and the composed stride map (dtype size 1) must be a bijection from
// (warp, lane) onto [0, 448).
//
// Notes: this is not bit-identical to the README's illustrative split (which
// peels warp's high digit first); both are valid bijections, and the
// implementation consistently peels the low digit.
func TestAccessReadmeExample(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	warp := f.Variable("bov", "warp", 14)
	lane := f.Variable("bov", "lane", 32)
	tile := NewConstHTuple([]Value{7, 16, 4})

	coord := mustAccess(t, env, []*Variable{warp, lane}, tile)
	if coord.Size() != 3 {
		t.Fatalf("coordinate rank = %d, want 3", coord.Size())
	}
	for i := 0; i < 3; i++ {
		if !coord.Index(i).IsLeaf() {
			t.Fatalf("coordinate dim %d is not a leaf stride", i)
		}
		if coord.Index(i).Leaf().Kind() != KindStride {
			t.Fatalf("coordinate dim %d kind = %d, want KindStride", i, coord.Index(i).Leaf().Kind())
		}
	}

	offset := mustStrideMap(t, env, &Layout{Coordinate: coord, Shape: tile}, 1)

	seen := make(map[int]bool, 448)
	for w := 0; w < 14; w++ {
		for l := 0; l < 32; l++ {
			assign := map[string]int{"warp": w, "lane": l}
			got := evalExpr(t, offset, assign)
			want := (w%7)*64 + (((w/7)%2)*8+l%8)*4 + l/8
			if got != want {
				t.Fatalf("offset(warp=%d, lane=%d) = %d, want %d", w, l, got, want)
			}
			if seen[got] {
				t.Fatalf("offset %d generated twice (warp=%d lane=%d)", got, w, l)
			}
			seen[got] = true
		}
	}
	if len(seen) != 448 {
		t.Errorf("got %d distinct offsets, want 448 (bijection)", len(seen))
	}
}

// TestStrideMapDtypeScale checks the byte-oriented scaling of the stride map.
// For a single variable t over tile [4, 6] the access map yields coordinates
// (t % 4, t / 4), so the element offset is (t%4)*6 + t/4.
func TestStrideMapDtypeScale(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	v := f.Variable("t", "t", 24)
	tile := NewConstHTuple([]Value{4, 6})

	coord := mustAccess(t, env, []*Variable{v}, tile)

	for _, dtype := range []int{1, 2, 4} {
		offset := mustStrideMap(t, env, &Layout{Coordinate: coord, Shape: tile}, dtype)
		for x := 0; x < 24; x++ {
			got := evalExpr(t, offset, map[string]int{"t": x})
			want := ((x%4)*6 + x/4) * dtype
			if got != want {
				t.Fatalf("dtype=%d x=%d: offset = %d, want %d", dtype, x, got, want)
			}
		}
	}

	// invalid dtype sizes are rejected
	if _, err := StrideMap(env, &Layout{Coordinate: coord, Shape: tile}, 3); err == nil {
		t.Error("StrideMap with dtypeSize 3: expected error")
	}
	if _, err := StrideMap(env, &Layout{Coordinate: coord, Shape: tile}, 0); err == nil {
		t.Error("StrideMap with dtypeSize 0: expected error")
	}
}

// TestAccessNestedTile exercises the recursive path of the access map: a tile
// whose first dimension is itself an htuple (2, 3).
//
//	BoV: single variable t with 0 <= t < 24
//	tile shape ((2, 3), 4)
//
// The flat coordinate is (t%6, t/6); recursion splits t%6 into
// ((t%6)%2, (t%6)/2). The composed element offset is
//
//	(((t%6)%2)*3 + (t%6)/2) * 4 + t/6
//
// which is a bijection onto [0, 24).
func TestAccessNestedTile(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	v := f.Variable("t", "t", 24)
	inner := NewConstHTuple([]Value{2, 3})
	tile := ConstHTuple{vec: []*ConstHTupleValue{
		NewConstHTupleValueHTuple(inner),
		NewConstHTupleValueLeaf(4),
	}}

	coord := mustAccess(t, env, []*Variable{v}, tile)
	if coord.Size() != 2 {
		t.Fatalf("coordinate rank = %d, want 2", coord.Size())
	}

	d0 := coord.Index(0)
	if !d0.IsHTuple() {
		t.Fatalf("coordinate dim 0 is not a nested htuple (kind %d)", d0.Kind())
	}
	if d0.HTuple().Size() != 2 {
		t.Fatalf("nested coordinate rank = %d, want 2", d0.HTuple().Size())
	}
	if !coord.Index(1).IsLeaf() {
		t.Fatalf("coordinate dim 1 is not a leaf")
	}

	layout := &Layout{Coordinate: coord, Shape: tile}
	offset := mustStrideMap(t, env, layout, 1)

	seen := make(map[int]bool, 24)
	for x := 0; x < 24; x++ {
		got := evalExpr(t, offset, map[string]int{"t": x})
		want := ((x%6)%2)*3*4 + ((x%6)/2)*4 + x/6
		if got != want {
			t.Fatalf("offset(t=%d) = %d, want %d", x, got, want)
		}
		if seen[got] {
			t.Fatalf("offset %d generated twice", got)
		}
		seen[got] = true
	}
}

// TestAccessDepth2NestedTile exercises two levels of htuple nesting in the tile
// shape: ((2, (2, 3)), 4) with a single variable t of bound 48.
func TestAccessDepth2NestedTile(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	v := f.Variable("t", "t", 48)
	sub := ConstHTuple{vec: []*ConstHTupleValue{
		NewConstHTupleValueLeaf(2),
		NewConstHTupleValueHTuple(NewConstHTuple([]Value{2, 3})),
	}}
	tile := ConstHTuple{vec: []*ConstHTupleValue{
		NewConstHTupleValueHTuple(sub),
		NewConstHTupleValueLeaf(4),
	}}

	coord := mustAccess(t, env, []*Variable{v}, tile)
	if coord.Size() != 2 {
		t.Fatalf("coordinate rank = %d, want 2", coord.Size())
	}
	if !coord.Index(0).IsHTuple() {
		t.Fatalf("coordinate dim 0 is not a nested htuple (kind %d)", coord.Index(0).Kind())
	}

	layout := &Layout{Coordinate: coord, Shape: tile}
	offset := mustStrideMap(t, env, layout, 1)

	seen := make(map[int]bool, 48)
	for x := 0; x < 48; x++ {
		got := evalExpr(t, offset, map[string]int{"t": x})
		want := (((x%12)%2)*6 + (((x%12)/2)%2)*3 + ((x%12)/4)%3) * 4
		want += x / 12
		if got != want {
			t.Fatalf("offset(t=%d) = %d, want %d", x, got, want)
		}
		if seen[got] {
			t.Fatalf("offset %d generated twice", got)
		}
		seen[got] = true
	}
}

// TestAccessBoVProductMismatch verifies that an over-sized bag of variables
// fails loudly instead of silently dropping the unconsumed high digits (which
// would make the mapping non-injective).
func TestAccessBoVProductMismatch(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	v := f.Variable("t", "t", 48) // BoV product 48...
	tile := NewConstHTuple([]Value{4, 6})

	if _, err := Access(env, []*Variable{v}, tile); err == nil { // ...tile rank 24
		t.Error("Access with BoV product 48 over tile rank 24: expected error")
	}
}

// TestAccessIncompatibleTile verifies that a tile extent whose factors no
// variable can cover is rejected.
func TestAccessIncompatibleTile(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	v := f.Variable("t", "t", 3) // 3 shares no factor with 4
	tile := NewConstHTuple([]Value{4})

	if _, err := Access(env, []*Variable{v}, tile); err == nil {
		t.Error("Access with incompatible tile: expected error")
	}
}

// TestAccessTrivialVariable verifies that a bound-1 variable is rejected up
// front: the GCD-based consumption can never consume it, so it would always
// fail the full-consumption invariant later with a confusing message.
func TestAccessTrivialVariable(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	one := f.Variable("t", "t", 1)
	other := f.Variable("t", "s", 12)
	tile := NewConstHTuple([]Value{12})

	if _, err := Access(env, []*Variable{one, other}, tile); err == nil {
		t.Error("Access with bound-1 variable: expected error")
	}
}
