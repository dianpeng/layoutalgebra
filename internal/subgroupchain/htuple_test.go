package subgroupchain

import "testing"

func leafHTuple(t *testing.T, env Env, values ...int) HTuple {
	t.Helper()
	f := env.Factory
	exprs := make([]Expr, 0, len(values))
	for _, v := range values {
		exprs = append(exprs, f.Const(v))
	}
	return NewHTuple(exprs)
}

func TestHTupleBasics(t *testing.T) {
	env := *NewEnv()
	h := leafHTuple(t, env, 2, 3, 4)

	if h.Size() != 3 {
		t.Fatalf("Size() = %d, want 3", h.Size())
	}
	for i, want := range []int{2, 3, 4} {
		v := h.Index(i)
		if !v.IsLeaf() {
			t.Fatalf("element %d is not a leaf", i)
		}
		if got := evalExpr(t, v.Leaf(), nil); got != want {
			t.Errorf("element %d = %d, want %d", i, got, want)
		}
	}

	if got := evalExpr(t, h.Rank(env), nil); got != 24 {
		t.Errorf("Rank() = %d, want 24", got)
	}

	// Set replaces an element
	f := env.Factory
	h.SetLeaf(1, f.Const(9))
	if got := evalExpr(t, h.Rank(env), nil); got != 72 {
		t.Errorf("Rank() after Set = %d, want 72", got)
	}
}

func TestHTupleFlatten(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	// (2, (3, 4), 5) => flatten => (2, 3*4, 5)
	inner := NewHTuple([]Expr{f.Const(3), f.Const(4)})
	h := HTuple{vec: []*HTupleValue{
		NewHTupleValueLeaf(f.Const(2)),
		NewHTupleValueHTuple(inner),
		NewHTupleValueLeaf(f.Const(5)),
	}}

	flat := h.Flatten(env)
	if flat.Size() != 3 {
		t.Fatalf("flatten size = %d, want 3", flat.Size())
	}
	if got := evalExpr(t, flat.Index(0).Leaf(), nil); got != 2 {
		t.Errorf("flat[0] = %d, want 2", got)
	}
	if got := evalExpr(t, flat.Index(1).Leaf(), nil); got != 12 {
		t.Errorf("flat[1] = %d, want 12", got)
	}
	if got := evalExpr(t, flat.Index(2).Leaf(), nil); got != 5 {
		t.Errorf("flat[2] = %d, want 5", got)
	}

	if h.Height() != 1 {
		t.Errorf("Height() = %d, want 1", h.Height())
	}
	if flat.Height() != 0 {
		t.Errorf("flat Height() = %d, want 0", flat.Height())
	}
}

func TestHTupleCloneIsDeep(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	inner := NewHTuple([]Expr{f.Const(3)})
	h := HTuple{vec: []*HTupleValue{
		NewHTupleValueLeaf(f.Const(2)),
		NewHTupleValueHTuple(inner),
	}}

	c := h.Clone()
	if c.Size() != h.Size() {
		t.Fatalf("clone size = %d, want %d", c.Size(), h.Size())
	}

	// mutating the original must not affect the clone
	h.Index(0).SetLeaf(f.Const(99))
	h.Index(1).HTuple().Index(0).SetLeaf(f.Const(99))

	if got := evalExpr(t, c.Index(0).Leaf(), nil); got != 2 {
		t.Errorf("clone[0] = %d after original mutation, want 2", got)
	}
	if got := evalExpr(t, c.Index(1).HTuple().Index(0).Leaf(), nil); got != 3 {
		t.Errorf("clone[1][0] = %d after original mutation, want 3", got)
	}
}

func TestHTupleValueAsHTuple(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	// a Tuple element converts to a flat HTuple of leaves
	tuple := Tuple{vec: []Expr{f.Const(2), f.Const(3)}}
	hv := NewHTupleValueTuple(tuple)
	asH := hv.AsHTuple()
	if asH.Size() != 2 || !asH.Index(0).IsLeaf() || !asH.Index(1).IsLeaf() {
		t.Fatalf("AsHTuple from tuple = %+v, want flat 2-leaf htuple", asH)
	}

	// a nested HTuple element clones through
	nested := NewHTuple([]Expr{f.Const(7)})
	hv2 := NewHTupleValueHTuple(nested)
	asH2 := hv2.AsHTuple()
	if asH2.Size() != 1 || evalExpr(t, asH2.Index(0).Leaf(), nil) != 7 {
		t.Fatalf("AsHTuple from htuple = %+v, want 1-leaf htuple of 7", asH2)
	}

	// a leaf must not convert
	defer func() {
		if recover() == nil {
			t.Error("AsHTuple on leaf: expected panic")
		}
	}()
	NewHTupleValueLeaf(f.Const(1)).AsHTuple()
}

func TestConstHTupleBasics(t *testing.T) {
	h := NewConstHTuple([]Value{2, 3, 4})

	if h.Size() != 3 {
		t.Fatalf("Size() = %d, want 3", h.Size())
	}
	if h.Rank() != 24 {
		t.Errorf("Rank() = %d, want 24", h.Rank())
	}
	for i, want := range []Value{2, 3, 4} {
		if got := h.Index(i).Leaf(); got != want {
			t.Errorf("element %d = %d, want %d", i, got, want)
		}
	}

	vec := h.ToVector()
	if len(vec) != 3 || vec[0] != 2 || vec[1] != 3 || vec[2] != 4 {
		t.Errorf("ToVector() = %v, want [2 3 4]", vec)
	}

	ct, ok := h.ToTuple()
	if !ok || ct.Rank() != 24 {
		t.Errorf("ToTuple() = %v (ok=%v), want rank 24", ct, ok)
	}

	h.SetLeaf(0, 5)
	if h.Index(0).Leaf() != 5 || h.Rank() != 60 {
		t.Errorf("after SetLeaf: [0]=%d rank=%d, want 5/60", h.Index(0).Leaf(), h.Rank())
	}
}

func TestConstHTupleFlatten(t *testing.T) {
	// (2, (3, 4), 5) => (2, 12, 5)
	inner := NewConstHTuple([]Value{3, 4})
	h := ConstHTuple{vec: []*ConstHTupleValue{
		NewConstHTupleValueLeaf(2),
		NewConstHTupleValueHTuple(inner),
		NewConstHTupleValueLeaf(5),
	}}

	flat := h.Flatten()
	vec := flat.ToVector()
	if len(vec) != 3 || vec[0] != 2 || vec[1] != 12 || vec[2] != 5 {
		t.Errorf("Flatten() = %v, want [2 12 5]", vec)
	}

	// ToVector on a recursive htuple fails
	if got := h.ToVector(); got != nil {
		t.Errorf("ToVector() on recursive = %v, want nil", got)
	}
	if _, ok := h.ToTuple(); ok {
		t.Error("ToTuple() on recursive: expected !ok")
	}

	if h.Height() != 1 {
		t.Errorf("Height() = %d, want 1", h.Height())
	}
}

func TestConstHTupleCloneIsDeep(t *testing.T) {
	inner := NewConstHTuple([]Value{3})
	h := ConstHTuple{vec: []*ConstHTupleValue{
		NewConstHTupleValueLeaf(2),
		NewConstHTupleValueHTuple(inner),
	}}

	c := h.Clone()
	h.Index(0).SetLeaf(99)
	h.Index(1).HTuple().Index(0).SetLeaf(99)

	if got := c.Index(0).Leaf(); got != 2 {
		t.Errorf("clone[0] = %d after mutation, want 2", got)
	}
	if got := c.Index(1).HTuple().Index(0).Leaf(); got != 3 {
		t.Errorf("clone[1][0] = %d after mutation, want 3", got)
	}
}
