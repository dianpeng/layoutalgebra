package subgroupchain

import "testing"

func TestTupleBasics(t *testing.T) {
	env := *NewEnv()
	f := env.Factory

	tpl := Tuple{vec: []Expr{f.Const(2), f.Const(3), f.Const(4)}}
	if tpl.Size() != 3 {
		t.Fatalf("Size() = %d, want 3", tpl.Size())
	}
	if got := evalExpr(t, tpl.Rank(env), nil); got != 24 {
		t.Errorf("Rank() = %d, want 24", got)
	}

	ct, err := tpl.ToConstTuple()
	if err != nil {
		t.Fatalf("ToConstTuple failed: %v", err)
	}
	if ct.Rank() != 24 || ct.Index(1) != 3 {
		t.Errorf("ToConstTuple rank = %d [1] = %d, want 24/3", ct.Rank(), ct.Index(1))
	}

	h := tpl.ToHTuple()
	if h.Size() != 3 || !h.Index(0).IsLeaf() {
		t.Errorf("ToHTuple size = %d, want 3 flat leaves", h.Size())
	}
}

func TestTupleToConstTupleError(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	v := f.Variable("t", "t", 8)

	tpl := Tuple{vec: []Expr{f.Const(2), v}}
	if _, err := tpl.ToConstTuple(); err == nil {
		t.Error("ToConstTuple with variable: expected error")
	}
}

func TestTupleGatherSliceClone(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	tpl := Tuple{vec: []Expr{f.Const(1), f.Const(2), f.Const(3), f.Const(4)}}

	g := tpl.Gather([]int{3, 1})
	if g.Size() != 2 || evalExpr(t, g.Index(0), nil) != 4 || evalExpr(t, g.Index(1), nil) != 2 {
		t.Errorf("Gather([3,1]) wrong")
	}

	s := tpl.Slice(1, 3)
	if s.Size() != 2 || evalExpr(t, s.Index(0), nil) != 2 || evalExpr(t, s.Index(1), nil) != 3 {
		t.Errorf("Slice(1,3) wrong")
	}

	c := tpl.Clone()
	tpl.Set(0, f.Const(99))
	if evalExpr(t, c.Index(0), nil) != 1 {
		t.Errorf("clone[0] = %d after mutation, want 1", evalExpr(t, c.Index(0), nil))
	}
}

func TestConstTupleBasics(t *testing.T) {
	ct := ConstTuple{vec: []Value{2, 3, 4}}
	if ct.Size() != 3 || ct.Rank() != 24 || ct.Index(2) != 4 {
		t.Errorf("basics wrong: size=%d rank=%d", ct.Size(), ct.Rank())
	}

	g := ct.Gather([]int{2, 0})
	if g.Size() != 2 || g.Index(0) != 4 || g.Index(1) != 2 {
		t.Errorf("Gather([2,0]) wrong")
	}

	s := ct.Slice(1, 3)
	if s.Size() != 2 || s.Index(0) != 3 || s.Index(1) != 4 {
		t.Errorf("Slice(1,3) wrong")
	}

	h := ct.ToHTuple()
	if h.Size() != 3 || h.Index(0).Leaf() != 2 {
		t.Errorf("ToHTuple wrong")
	}

	c := ct.Clone()
	ct.Set(0, 99)
	if c.Index(0) != 2 {
		t.Errorf("clone[0] = %d after mutation, want 2", c.Index(0))
	}
}
