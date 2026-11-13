package f2

import (
	"strings"
	"testing"
)

// Regression tests for the 15 warning findings of 01-REVIEW.md.
// Each test exercises the exact input the reviewer used to confirm the bug.

// mustPanicContains asserts that f panics with a message containing want.
func mustPanicContains(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q, got none", want)
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T (%v), want string message", r, r)
		}
		if !strings.Contains(msg, want) {
			t.Fatalf("panic message %q does not contain %q", msg, want)
		}
	}()
	f()
}

// ---------------------------------------------------------------------------
// WR-01: doCoeffMat never folded the symbolized expression — duplicate
// same-variable closures silently overwrote each other
// ---------------------------------------------------------------------------

func TestFixWR01CoeffMatFold(t *testing.T) {
	x := NewVariable("x")
	m := NewSymbol("M")
	n := NewSymbol("N")
	v := NewVector([]Expr{
		NewBinary(Add, NewBinary(Mul, m, x), NewBinary(Mul, n, x)),
	})

	mat, err := v.CoeffMatAll()
	if err != nil {
		t.Fatalf("CoeffMatAll([(M*x)+(N*x)]) error: %v", err)
	}
	want := Fold(NewBinary(Add, m, n)) // canonical ordering on the want side
	if !mat.Index(0, 0).IsEqual(want) {
		t.Errorf("coefficient of x = %v, want Fold(M+N)", mat.Index(0, 0))
	}
}

// ---------------------------------------------------------------------------
// WR-02: ShiftR (and ShiftL) left vacated positions as nil Expr holes
// ---------------------------------------------------------------------------

func TestFixWR02ShiftZeroFill(t *testing.T) {
	a := NewVariable("a")
	b := NewVariable("b")
	v := NewVector([]Expr{a, b})

	shiftedR := v.ShiftR(1, true)
	if shiftedR.Size() != 3 {
		t.Fatalf("ShiftR(1, true) size = %d, want 3", shiftedR.Size())
	}
	mustConst(t, shiftedR.Index(0), Zero)
	if !shiftedR.Index(1).IsEqual(a) || !shiftedR.Index(2).IsEqual(b) {
		t.Errorf("ShiftR(1, true) = [%v %v %v], want [0 a b]",
			shiftedR.Index(0), shiftedR.Index(1), shiftedR.Index(2))
	}

	shiftedL := v.ShiftL(1, true)
	if shiftedL.Size() != 3 {
		t.Fatalf("ShiftL(1, true) size = %d, want 3", shiftedL.Size())
	}
	if !shiftedL.Index(0).IsEqual(a) || !shiftedL.Index(1).IsEqual(b) {
		t.Errorf("ShiftL(1, true) = [%v %v %v], want [a b 0]",
			shiftedL.Index(0), shiftedL.Index(1), shiftedL.Index(2))
	}
	mustConst(t, shiftedL.Index(2), Zero)

	// every slot must be a real Expr — a nil hole nil-panics on .Kind()
	for i := 0; i < shiftedR.Size(); i++ {
		_ = shiftedR.Index(i).Kind()
	}
	for i := 0; i < shiftedL.Size(); i++ {
		_ = shiftedL.Index(i).Kind()
	}
}

// ---------------------------------------------------------------------------
// WR-03: IsPerm did not check column uniqueness
// ---------------------------------------------------------------------------

func TestFixWR03IsPermColumnUniqueness(t *testing.T) {
	dup := constCoeffMat(t, [][]F2{{One, Zero}, {One, Zero}})
	if dup.IsPerm() {
		t.Error("IsPerm on [[1,0],[1,0]] = true, want false (column 0 claimed twice)")
	}

	swap := constCoeffMat(t, [][]F2{{Zero, One}, {One, Zero}})
	if !swap.IsPerm() {
		t.Error("IsPerm on [[0,1],[1,0]] = false, want true")
	}
}

// ---------------------------------------------------------------------------
// WR-04: IsIdentity panicked on non-square matrices instead of returning false
// ---------------------------------------------------------------------------

func TestFixWR04IsIdentityNonSquare(t *testing.T) {
	// 2x3 diagonal-1 shape: pre-fix returned true (false assertion path)
	wide := constCoeffMat(t, [][]F2{{One, Zero, Zero}, {Zero, One, Zero}})
	if wide.IsIdentity() {
		t.Error("IsIdentity on 2x3 [[1,0,0],[0,1,0]] = true, want false (non-square)")
	}

	// 3x2 shape: pre-fix panicked (m > n indexes past the row length)
	tall := constCoeffMat(t, [][]F2{{One, Zero}, {Zero, One}, {Zero, Zero}})
	if tall.IsIdentity() {
		t.Error("IsIdentity on 3x2 [[1,0],[0,1],[0,0]] = true, want false (non-square)")
	}
}

// ---------------------------------------------------------------------------
// WR-08: Vector.Add had no size check (panic or silent truncation)
// ---------------------------------------------------------------------------

func TestFixWR08VectorAddSizeMismatch(t *testing.T) {
	a := NewVariable("a")
	b := NewVariable("b")
	c := NewVariable("c")
	v := NewVector([]Expr{a, b})

	mustPanicContains(t, "CHECK", func() {
		v.Add(NewVector([]Expr{c}))
	})

	// equal sizes still work
	d := NewVariable("d")
	out := v.Add(NewVector([]Expr{c, d}))
	if out.Size() != 2 {
		t.Fatalf("Add size = %d, want 2", out.Size())
	}
}

// ---------------------------------------------------------------------------
// WR-09: PermutationMatrix panicked on rank mismatch and missed non-bijective
// inputs (duplicate rhs match on one lhs column)
// ---------------------------------------------------------------------------

func TestFixWR09PermutationMatrixErrors(t *testing.T) {
	a := NewVariable("a")
	b := NewVariable("b")

	// rank mismatch: pre-fix this panicked despite the error-returning API
	if _, err := PermutationMatrix(NewVector([]Expr{a}), NewVector([]Expr{a, b})); err == nil {
		t.Error("PermutationMatrix rank mismatch: err = nil, want non-nil")
	}

	// rhs=[a,a], lhs=[a,b]: pre-fix silently produced a non-permutation matrix
	if _, err := PermutationMatrix(
		NewVector([]Expr{a, b}),
		NewVector([]Expr{a, a}),
	); err == nil {
		t.Error("PermutationMatrix duplicate match: err = nil, want non-nil")
	}

	// the valid bijective case still works
	if _, err := PermutationMatrix(
		NewVector([]Expr{a, b}),
		NewVector([]Expr{b, a}),
	); err != nil {
		t.Errorf("PermutationMatrix valid swap: err = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// WR-11: NewFromSemanticMapping indexed namePrefix[dim] without a length check
// ---------------------------------------------------------------------------

func TestFixWR11SemanticMappingPrefixGuard(t *testing.T) {
	mustPanicContains(t, "CHECK", func() {
		NewFromSemanticMapping([][]int{{1, 1}}, []string{}) // 1 dim, 0 prefixes
	})
}

// ---------------------------------------------------------------------------
// WR-15: ShiftMatrix panicked on negative k
// ---------------------------------------------------------------------------

func TestFixWR15ShiftMatrixNegativeK(t *testing.T) {
	mustPanicContains(t, "CHECK", func() {
		ShiftMatrix(3, -1)
	})
}

// ---------------------------------------------------------------------------
// WR-05: swizzle free-bit scan started at bank.Range + 1 instead of
// bank.Skip + bank.Range — a bank bit was eligible as a free XOR source
// ---------------------------------------------------------------------------

func TestFixWR05FreeScanStart(t *testing.T) {
	l0 := NewVariable("l_0")
	l1 := NewVariable("l_1")
	l2 := NewVariable("l_2")
	l3 := NewVariable("l_3")
	// 8-bit input, bank bits at [2, 7) = [l_0, l_0, l_1, l_2, l_3]; the only
	// candidate past the bank is index 7, which is ConstZero. Pre-fix the scan
	// started at index 6 (inside the bank) and used bank bit l_3 as the free
	// XOR source; post-fix no free degree exists.
	input := NewVector([]Expr{
		NewConstZero(), NewConstZero(), l0, l0, l1, l2, l3, NewConstZero(),
	})
	bank := Bank{Skip: 2, Range: 5, VariablePrefix: "l"}

	if _, err := SwizzleMap(bank, input); err != SwizzleErrorNoFreeDegree {
		t.Errorf("err = %v, want SwizzleErrorNoFreeDegree", err)
	}
}

// ---------------------------------------------------------------------------
// WR-06: empty free vector panicked inside NewCoeffMat instead of returning
// SwizzleErrorNoFreeDegree
// ---------------------------------------------------------------------------

func TestFixWR06NoFreeDegree(t *testing.T) {
	// bank rank 1 < Range 2, no family bits past the bank -> no free degree
	input := NewVector([]Expr{
		NewVariable("t0"), NewVariable("t0"), NewVariable("w0"),
	})
	bank := Bank{Skip: 0, Range: 2, VariablePrefix: "t"}

	if _, err := SwizzleMap(bank, input); err != SwizzleErrorNoFreeDegree {
		t.Errorf("err = %v, want SwizzleErrorNoFreeDegree", err)
	}
}

// ---------------------------------------------------------------------------
// WR-07: input.Slice(bank.Skip, bank.Skip+bank.Range) was unchecked
// ---------------------------------------------------------------------------

func TestFixWR07BankBounds(t *testing.T) {
	input := NewVector([]Expr{
		NewVariable("t0"), NewVariable("t0"), NewVariable("w0"), NewVariable("w0"),
	})
	bank := Bank{Skip: 3, Range: 5, VariablePrefix: "t"} // 3+5 > 4

	if _, err := SwizzleMap(bank, input); err == nil {
		t.Error("err = nil, want non-nil error for Skip+Range > input size")
	}
}

// ---------------------------------------------------------------------------
// WR-10: fold driver silently returned a non-canonical expression when the
// iteration cap was exhausted
// ---------------------------------------------------------------------------

func TestFixWR10FoldCapExhausted(t *testing.T) {
	x := NewVariable("x")
	one := NewConstOne()
	// ((x + 1) + 1) needs at least 2 rewrite rounds (commute at the inner add,
	// then commute/const-fold at the root), so maxBound: 1 exhausts the cap
	// with a non-empty replacement queue.
	input := NewBinary(Add, NewBinary(Add, x, one), one)

	f := &folder{maxBound: 1}
	mustPanicContains(t, "BUG", func() {
		f.run(input)
	})

	// ordinary Fold on the same input still converges to canonical form (x)
	if out := Fold(input); !out.IsEqual(x) {
		t.Errorf("Fold(((x+1)+1)) = %v, want x", out)
	}
}

// ---------------------------------------------------------------------------
// WR-12: StrideMap did not validate dtypeSize
// ---------------------------------------------------------------------------

func TestFixWR12StrideMapDtypeSize(t *testing.T) {
	a := NewVariable("a")
	b := NewVariable("b")
	v := NewVector([]Expr{a, b})
	tileShape := []int{0, 0}

	if _, err := StrideMap(tileShape, v, 3); err == nil {
		t.Error("StrideMap(dtypeSize=3) err = nil, want non-nil")
	}

	for _, size := range []int{0, 1, 2} {
		out, err := StrideMap(tileShape, v, size)
		if err != nil {
			t.Errorf("StrideMap(dtypeSize=%d) err = %v, want nil", size, err)
			continue
		}
		if want := 2 + size; out.Size() != want {
			t.Errorf("StrideMap(dtypeSize=%d) size = %d, want %d", size, out.Size(), want)
		}
	}
}

// ---------------------------------------------------------------------------
// WR-13: NameBelong prefix matching was fragile across variable families
// ---------------------------------------------------------------------------

func TestFixWR13NameBelong(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		want   bool
	}{
		{"thr1_0", "thr1", true},   // same family
		{"thr10_0", "thr1", false}, // digit-ending prefix: "0" continues family thr10
		{"t0", "t", true},          // hardware convention
		{"t_0", "t", true},         // underscore boundary
		{"thr1_0", "thr", true},    // digit after non-digit prefix is a new dim
		{"t", "t", true},           // exact match
		{"t0", "w", false},         // different prefix
	}
	for _, c := range cases {
		if got := NameBelong(c.name, c.prefix); got != c.want {
			t.Errorf("NameBelong(%q, %q) = %v, want %v", c.name, c.prefix, got, c.want)
		}
	}
}
