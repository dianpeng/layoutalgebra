package f2

import "testing"

// constCoeffMat builds an m-by-n CoeffMat from a raw 0/1 pattern.
func constCoeffMat(t *testing.T, raw [][]F2) CoeffMat {
  t.Helper()
  cm := NewConstMat(raw)
  return cm.ToCoeffMat()
}

// TestCoeffMatApplyIdentity proves the F2 matrix-vector product semantics:
// the identity matrix maps [x, y] to [x, y]. With the (buggy) swapped
// operator roles the diagonal would compute x*x + ... via AND-reduce and
// this test fails — map must be Mul(AND), reduce must be Add(XOR).
func TestCoeffMatApplyIdentity(t *testing.T) {
  mat := constCoeffMat(t, [][]F2{
    {One, Zero},
    {Zero, One},
  })
  vec := NewVector([]Expr{NewVariable("x"), NewVariable("y")})

  got := mat.Apply(vec)
  if got.Rank() != 2 {
    t.Fatalf("Apply(identity).Rank() = %d, want 2", got.Rank())
  }
  if !got.Extent(0).IsEqual(NewVariable("x")) {
    t.Errorf("Apply(identity)[0] structurally non-equal to x")
  }
  if !got.Extent(1).IsEqual(NewVariable("y")) {
    t.Errorf("Apply(identity)[1] structurally non-equal to y")
  }
}

// TestCoeffMatApplySwap proves map=Mul/reduce=Add via the swap permutation:
// [[0,1],[1,0]] applied to [x, y] must yield [y, x].
func TestCoeffMatApplySwap(t *testing.T) {
  mat := constCoeffMat(t, [][]F2{
    {Zero, One},
    {One, Zero},
  })
  vec := NewVector([]Expr{NewVariable("x"), NewVariable("y")})

  got := mat.Apply(vec)
  if got.Rank() != 2 {
    t.Fatalf("Apply(swap).Rank() = %d, want 2", got.Rank())
  }
  if !got.Extent(0).IsEqual(NewVariable("y")) {
    t.Errorf("Apply(swap)[0] structurally non-equal to y")
  }
  if !got.Extent(1).IsEqual(NewVariable("x")) {
    t.Errorf("Apply(swap)[1] structurally non-equal to x")
  }
}

// TestCoeffMatApplyZero proves the zero matrix annihilates the vector.
func TestCoeffMatApplyZero(t *testing.T) {
  mat := NewCoeffMatAllZero(2, 2)
  vec := NewVector([]Expr{NewVariable("x"), NewVariable("y")})

  got := mat.Apply(vec)
  if got.Rank() != 2 {
    t.Fatalf("Apply(zero).Rank() = %d, want 2", got.Rank())
  }
  if !got.Extent(0).IsEqual(NewConstZero()) {
    t.Errorf("Apply(zero)[0] structurally non-equal to Const0")
  }
  if !got.Extent(1).IsEqual(NewConstZero()) {
    t.Errorf("Apply(zero)[1] structurally non-equal to Const0")
  }
}
