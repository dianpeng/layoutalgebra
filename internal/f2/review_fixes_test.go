package f2

import "testing"

// Regression tests for the seven critical findings of 01-REVIEW.md.
// Each test exercises the exact input the reviewer used to confirm the bug.

// mustConst asserts that e is the constant node with value want.
func mustConst(t *testing.T, e Expr, want F2) {
	t.Helper()
	c, ok := e.(*NodeConst)
	if !ok {
		t.Fatalf("expected *NodeConst, got %T (%v)", e, e)
	}
	if c.Value() != want {
		t.Fatalf("const value = %v, want %v", c.Value(), want)
	}
}

// ---------------------------------------------------------------------------
// CR-01: CoeffMat.ToConst never appended rows — always returned an empty matrix
// ---------------------------------------------------------------------------

func TestToConstReturnsRows(t *testing.T) {
	sym := constCoeffMat(t, [][]F2{
		{One, Zero},
		{Zero, One},
	})

	cm, err := sym.ToConst()
	if err != nil {
		t.Fatalf("ToConst on constant matrix returned error: %v", err)
	}
	m, n := cm.Size()
	if m != 2 || n != 2 {
		t.Fatalf("ToConst size = %dx%d, want 2x2", m, n)
	}
	mustConst(t, cm.Index(0, 0), One)
	mustConst(t, cm.Index(0, 1), Zero)
	mustConst(t, cm.Index(1, 0), Zero)
	mustConst(t, cm.Index(1, 1), One)
}

func TestToConstRejectsSymbolicMatrix(t *testing.T) {
	sym := NewCoeffMat([][]Expr{{NewVariable("x")}})
	if _, err := sym.ToConst(); err == nil {
		t.Fatal("ToConst on a matrix containing a variable must return an error")
	}
}

// ---------------------------------------------------------------------------
// CR-02: Gaussian elimination rank formula was inverted (complement of rank)
// ---------------------------------------------------------------------------

func TestGaussianEliminationRank(t *testing.T) {
	cases := []struct {
		name string
		raw  [][]F2
		want int
	}{
		{"identity 2x2", [][]F2{{One, Zero}, {Zero, One}}, 2},
		{"single nonzero row", [][]F2{{One, Zero}, {Zero, Zero}, {Zero, Zero}}, 1},
		{"zero matrix", [][]F2{{Zero, Zero}, {Zero, Zero}}, 0},
		{"full 3x3", [][]F2{
			{One, Zero, Zero},
			{Zero, One, Zero},
			{Zero, Zero, One},
		}, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := constCoeffMat(t, c.raw)
			res, err := GuassianEliminationConstMat(in)
			if err != nil {
				t.Fatalf("elimination error: %v", err)
			}
			if res.Rank != c.want {
				t.Errorf("rank = %d, want %d", res.Rank, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CR-03: elimination kept a stale pivot row after a row swap
// ---------------------------------------------------------------------------

func TestGaussianEliminationRowSwap(t *testing.T) {
	// First pivot is zero — forces a swap on the very first column.
	in := constCoeffMat(t, [][]F2{
		{Zero, One},
		{One, Zero},
	})
	res, err := GuassianEliminationConstMat(in)
	if err != nil {
		t.Fatalf("elimination error: %v", err)
	}
	if res.Rank != 2 {
		t.Errorf("rank = %d, want 2", res.Rank)
	}
}

func TestGaussianEliminationDependentRows(t *testing.T) {
	// Over F2 the three rows sum to zero, so the true rank is 2.
	// With the stale-pivot bug this reported rank 3.
	in := constCoeffMat(t, [][]F2{
		{Zero, One, One},
		{One, Zero, One},
		{One, One, Zero},
	})
	res, err := GuassianEliminationConstMat(in)
	if err != nil {
		t.Fatalf("elimination error: %v", err)
	}
	if res.Rank != 2 {
		t.Errorf("rank = %d, want 2", res.Rank)
	}
	// the eliminated last row must be all zeros
	for j := 0; j < 3; j++ {
		mustConst(t, res.Matrix.Index(2, j), Zero)
	}
}

// ---------------------------------------------------------------------------
// CR-04: coefficient extraction panicked on a bare variable (coefficient 1)
// ---------------------------------------------------------------------------

func TestCoeffMatAllBareVariables(t *testing.T) {
	v := NewVector([]Expr{NewVariable("x"), NewVariable("y")})
	mat, err := v.CoeffMatAll()
	if err != nil {
		t.Fatalf("CoeffMatAll([x, y]) error: %v", err)
	}
	// must yield the 2x2 identity
	m, n := mat.Size()
	if m != 2 || n != 2 {
		t.Fatalf("size = %dx%d, want 2x2", m, n)
	}
	mustConst(t, mat.Index(0, 0), One)
	mustConst(t, mat.Index(0, 1), Zero)
	mustConst(t, mat.Index(1, 0), Zero)
	mustConst(t, mat.Index(1, 1), One)
}

// ---------------------------------------------------------------------------
// CR-05: coefficient extraction demanded the wrong tree shape — panicked on
// the canonical folded form S*x and silently dropped factors otherwise
// ---------------------------------------------------------------------------

func TestCoeffMatAllSymbolTimesVariable(t *testing.T) {
	x := NewVariable("x")
	s := NewSymbol("S")
	v := NewVector([]Expr{NewBinary(Mul, s, x)}) // canonical folded form: S*x

	mat, err := v.CoeffMatAll()
	if err != nil {
		t.Fatalf("CoeffMatAll([S*x]) error: %v", err)
	}
	m, n := mat.Size()
	if m != 1 || n != 1 {
		t.Fatalf("size = %dx%d, want 1x1", m, n)
	}
	if !mat.Index(0, 0).IsEqual(s) {
		t.Errorf("coefficient of S*x = %v, want S", mat.Index(0, 0))
	}
}

func TestCoeffMatAllCompoundCoefficient(t *testing.T) {
	x := NewVariable("x")
	s1 := NewSymbol("S1")
	s2 := NewSymbol("S2")
	// left-associative product chain, variable innermost-right
	v := NewVector([]Expr{
		NewBinary(Mul, NewBinary(Mul, s1, s2), x),
	})

	mat, err := v.CoeffMatAll()
	if err != nil {
		t.Fatalf("CoeffMatAll([(S1*S2)*x]) error: %v", err)
	}
	want := NewBinary(Mul, s1, s2)
	if !mat.Index(0, 0).IsEqual(want) {
		t.Errorf("coefficient = %v, want S1*S2", mat.Index(0, 0))
	}
}

// ---------------------------------------------------------------------------
// CR-06: ShiftL shifted by 1 regardless of k and panicked at k=0
// ---------------------------------------------------------------------------

func TestShiftLWideningPositions(t *testing.T) {
	a := NewVariable("a")
	b := NewVariable("b")
	v := NewVector([]Expr{a, b})

	shifted := v.ShiftL(2, true) // reserve 2 low slots, elements keep position
	if shifted.Size() != 4 {
		t.Fatalf("ShiftL(2, true) size = %d, want 4", shifted.Size())
	}
	if !shifted.Index(0).IsEqual(a) {
		t.Errorf("position 0 = %v, want a", shifted.Index(0))
	}
	if !shifted.Index(1).IsEqual(b) {
		t.Errorf("position 1 = %v, want b", shifted.Index(1))
	}
}

func TestShiftLZeroNoPanic(t *testing.T) {
	a := NewVariable("a")
	b := NewVariable("b")
	v := NewVector([]Expr{a, b})

	shifted := v.ShiftL(0, true) // must not panic (previously index-out-of-range)
	if shifted.Size() != 2 {
		t.Fatalf("ShiftL(0, true) size = %d, want 2", shifted.Size())
	}
	if !shifted.Index(0).IsEqual(a) || !shifted.Index(1).IsEqual(b) {
		t.Error("ShiftL(0, true) must leave the vector unchanged")
	}
}

// ---------------------------------------------------------------------------
// CR-07: swizzle indexed the (short) free vector with absolute input-space
// indices — the first XOR panicked with index out of range
// ---------------------------------------------------------------------------

func TestSwizzleMapEndToEnd(t *testing.T) {
	t0 := NewVariable("t0")
	t1 := NewVariable("t1")
	// bank bits [t0, t0]: rank 1 of max 2 -> not saturated, nullspace = {1};
	// index 3 (family "t") is the free degree; index 2 is ineligible filler.
	input := NewVector([]Expr{t0, t0, NewConstZero(), t1})
	bank := Bank{Skip: 0, Range: 2, VariablePrefix: "t"}

	out, err := SwizzleMap(bank, input)
	if err != nil {
		t.Fatalf("SwizzleMap error: %v", err)
	}
	if out.Size() != 4 {
		t.Fatalf("output size = %d, want 4", out.Size())
	}
	// untouched slots
	if !out.Index(0).IsEqual(t0) {
		t.Errorf("slot 0 = %v, want t0", out.Index(0))
	}
	if !out.Index(3).IsEqual(t1) {
		t.Errorf("slot 3 = %v, want t1", out.Index(3))
	}
	// swizzled slot: t0 ^ t1
	want := NewBinary(Xor, t0, t1)
	if !out.Index(1).IsEqual(want) {
		t.Errorf("slot 1 = %v, want %v", out.Index(1), want)
	}
}
