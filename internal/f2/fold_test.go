package f2

import "testing"

// TestFoldIdentities proves ALG-02's six F2 fold identities plus full-constant
// folding. Each case: input tree -> expected structurally-equal tree after Fold.
func TestFoldIdentities(t *testing.T) {
	x := NewVariable("x")

	cases := []struct {
		name  string
		input Expr
		want  Expr
	}{
		{"xor_x_0_to_x", NewAdd(x, NewConstZero()), x},
		{"xor_0_x_to_x", NewAdd(NewConstZero(), x), x},
		{"xor_x_x_to_0", NewAdd(x, x), NewConstZero()},
		{"and_x_0_to_0", NewMul(x, NewConstZero()), NewConstZero()},
		{"and_x_1_to_x", NewMul(x, NewConstOne()), x},
		{"and_x_x_to_x", NewMul(x, x), x},
		{"xor_1_1_to_0", NewAdd(NewConstOne(), NewConstOne()), NewConstZero()},
		{"and_1_1_to_1", NewMul(NewConstOne(), NewConstOne()), NewConstOne()},
		{"xor_0_0_to_0", NewAdd(NewConstZero(), NewConstZero()), NewConstZero()},
		{"and_0_1_to_0", NewMul(NewConstZero(), NewConstOne()), NewConstZero()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Fold(c.input)
			if !got.IsEqual(c.want) {
				t.Errorf("Fold identity %s: got non-canonical result (want structurally %v)", c.name, c.want)
			}
		})
	}
}

// TestFoldNormalization proves the rewriteNormalization rule's math:
// S1*x + S2*x => (S1+S2)*x — the two COEFFICIENTS combine, not coefficient+variable.
func TestFoldNormalization(t *testing.T) {
	x := NewVariable("x")
	s1 := NewSymbol("S1")
	s2 := NewSymbol("S2")

	input := NewAdd(NewMul(s1, x), NewMul(s2, x))
	want := NewMul(NewAdd(NewSymbol("S1"), NewSymbol("S2")), x)

	got := Fold(input)
	if !got.IsEqual(want) {
		t.Errorf("Fold(S1*x + S2*x): got structurally non-equal to (S1+S2)*x")
	}
}

// TestFoldNormalizationBareVariable pins the bare-variable coefficient-1
// special case of rewriteNormalization: C1*v + C2*v => (C1+C2)*v must also
// fire when one side is the bare variable (C=1). Both operand orders must
// converge to the SAME canonical tree Mul(Add(One, S), x). rewriteCommute
// cannot be relied on to normalize the order (a Mul is not fully static), so
// each order needs its own normalization branch.
func TestFoldNormalizationBareVariable(t *testing.T) {
	x := NewVariable("x")
	s := NewSymbol("S")
	want := NewMul(NewAdd(NewConstOne(), s), x)

	cases := []struct {
		name  string
		input Expr
	}{
		{"x_plus_Sx", NewAdd(x, NewMul(s, x))},
		{"Sx_plus_x", NewAdd(NewMul(s, x), x)},
		// regression locks: pre-existing shapes must stay canonical
		{"Mx_plus_Nx", NewAdd(NewMul(NewSymbol("M"), x), NewMul(NewSymbol("N"), x))},
		{"x_plus_x", NewAdd(x, NewVariable("x"))},
	}
	wants := []Expr{
		want,
		want,
		NewMul(NewAdd(NewSymbol("M"), NewSymbol("N")), x),
		NewConstZero(),
	}

	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Fold(c.input)
			if !got.IsEqual(wants[i]) {
				t.Errorf("Fold(%s): got structurally non-equal to %v", c.name, wants[i])
			}
		})
	}
}

// TestFoldNormalizationBareVariableNegative guards the merge's firing
// conditions: a Mul over a DIFFERENT variable, two bare variables, and a
// non-static-coefficient shape must all pass through untouched.
func TestFoldNormalizationBareVariableNegative(t *testing.T) {
	x := NewVariable("x")
	y := NewVariable("y")
	s := NewSymbol("S")

	cases := []struct {
		name  string
		input Expr
	}{
		{"mul_on_other_variable", NewAdd(x, NewMul(s, y))},
		{"two_bare_variables", NewAdd(x, y)},
		{"non_static_coefficient", NewAdd(x, NewMul(s, NewAdd(x, y)))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Fold(c.input); !got.IsEqual(c.input) {
				t.Errorf("Fold(%s): got structurally non-equal to unchanged input %v", c.name, c.input)
			}
		})
	}
}

// TestFoldNormalizationBareVariableConst pins the constant coefficient-1
// merge: x + 1*x collapses fully to 0 (merged coefficient 1+1 folds to 0,
// then Mul(0, v) folds to 0), in both operand orders.
func TestFoldNormalizationBareVariableConst(t *testing.T) {
	x := NewVariable("x")

	cases := []struct {
		name  string
		input Expr
	}{
		{"x_plus_1x", NewAdd(x, NewMul(NewConstOne(), x))},
		{"1x_plus_x", NewAdd(NewMul(NewConstOne(), x), x)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Fold(c.input); !got.IsEqual(NewConstZero()) {
				t.Errorf("Fold(%s): got structurally non-equal to Const0", c.name)
			}
		})
	}
}

// TestFoldNormalizationCertificateSoundness is the integration lock for the
// x + M*x silent-drop: extractLinearCoeffVector relies on Fold to merge
// per-variable closures. Before the bare-variable merge, Fold left
// x + M*x unmerged and the extractor's exprMap overwrite silently kept only
// the M*x closure, returning coefficient M instead of 1+M. The fold-layer
// fix must make CoeffMatAll return the single merged coefficient (1+M).
func TestFoldNormalizationCertificateSoundness(t *testing.T) {
	x := NewVariable("x")
	m := NewSymbol("M")

	v := NewVector([]Expr{NewAdd(x, NewMul(m, x))})
	mat, err := v.CoeffMatAll()
	if err != nil {
		t.Fatalf("CoeffMatAll([x + M*x]): unexpected error %v", err)
	}
	rows, cols := mat.Size()
	if rows != 1 || cols != 1 {
		t.Fatalf("CoeffMatAll([x + M*x]): size = %dx%d, want 1x1", rows, cols)
	}
	want := NewAdd(NewConstOne(), m)
	if got := mat.Index(0, 0); !got.IsEqual(want) {
		t.Errorf("CoeffMatAll([x + M*x]) coefficient: got structurally non-equal to %v (merged 1+M, not silently M)", want)
	}
}

// TestFoldConstantSubtree proves an inner constant-only subtree folds to a Const
// and then participates in the outer identity: Xor(And(1,0), x) -> x.
func TestFoldConstantSubtree(t *testing.T) {
	x := NewVariable("x")
	input := NewAdd(NewMul(NewConstOne(), NewConstZero()), x)

	got := Fold(input)
	if !got.IsEqual(x) {
		t.Errorf("Fold(Xor(And(1,0), x)): got structurally non-equal to x")
	}
}

// TestFoldCompositeIdempotent proves two identical composite operands cancel
// under XOR: Xor(x+y, x+y) -> Const0.
func TestFoldCompositeIdempotent(t *testing.T) {
	x := NewVariable("x")
	y := NewVariable("y")

	input := NewAdd(NewAdd(x, y), NewAdd(NewVariable("x"), NewVariable("y")))
	got := Fold(input)
	if !got.IsEqual(NewConstZero()) {
		t.Errorf("Fold(Xor(x+y, x+y)): got structurally non-equal to Const0")
	}
}

// TestFoldDeterministic proves ALG-02's determinism requirement: folding two
// separately-constructed identical trees yields structurally equal results.
// With the old map-iteration driver this could differ between runs.
func TestFoldDeterministic(t *testing.T) {
	build := func() Expr {
		return NewAdd(
			NewMul(NewConstOne(), NewVariable("x")),
			NewAdd(NewVariable("y"), NewVariable("y")),
		)
	}

	got1 := Fold(build())
	got2 := Fold(build())

	if !got1.IsEqual(got2) {
		t.Errorf("Fold is nondeterministic: two identical inputs folded to structurally different results")
	}

	// sanity: the constant fold (1*x -> x), commute and idempotent (y+y -> 0)
	// cases all fired, leaving just x
	if !got1.IsEqual(NewVariable("x")) {
		t.Errorf("Fold(Xor(And(1,x), Xor(y,y))): got structurally non-equal to x")
	}
}

// TestFoldNoAliasing proves the tree-never-DAG invariant for fold: folding
// Xor(Const0, v) returns a result equal to v, and the returned tree contains
// exactly the variables of the input (nothing dropped, nothing foreign).
func TestFoldNoAliasing(t *testing.T) {
	v := NewVariable("v")
	input := NewAdd(NewConstZero(), v)

	got := Fold(input)
	if !got.IsEqual(NewVariable("v")) {
		t.Errorf("Fold(Xor(0, v)): got structurally non-equal to v")
	}

	names := GetExprVariableNameSet(got)
	if len(names) != 1 || names[0] != "v" {
		t.Errorf("Fold(Xor(0, v)) variable set = %v, want [v]", names)
	}
}
