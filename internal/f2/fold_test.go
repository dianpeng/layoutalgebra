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
