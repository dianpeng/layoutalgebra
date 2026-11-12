package f2

import "testing"

// mustEqual asserts that got and want are structurally equal (IsEqual).
// Local helper — kept tiny per project convention (stdlib only, no assertion libs).
func mustEqual(t *testing.T, got, want Expr) {
  t.Helper()
  if !got.IsEqual(want) {
    t.Fatalf("structural mismatch: got %v, want %v", got, want)
  }
}

// mustNotEqual asserts that got and want are NOT structurally equal.
func mustNotEqual(t *testing.T, got, want Expr) {
  t.Helper()
  if got.IsEqual(want) {
    t.Fatalf("unexpected structural equality: got %v, want %v (must differ)", got, want)
  }
}

// TestF2Add locks in ALG-01's scalar half: Add over F2 is XOR.
func TestF2Add(t *testing.T) {
  cases := []struct {
    a, b F2
    want F2
  }{
    {Zero, Zero, Zero},
    {Zero, One, One},
    {One, Zero, One},
    {One, One, Zero},
  }
  for _, c := range cases {
    if got := c.a.Add(c.b); got != c.want {
      t.Errorf("Add(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
    }
  }
}

// TestF2Mul locks in ALG-01's scalar half: Mul over F2 is AND.
func TestF2Mul(t *testing.T) {
  cases := []struct {
    a, b F2
    want F2
  }{
    {Zero, Zero, Zero},
    {Zero, One, Zero},
    {One, Zero, Zero},
    {One, One, One},
  }
  for _, c := range cases {
    if got := c.a.Mul(c.b); got != c.want {
      t.Errorf("Mul(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
    }
  }
}

// TestF2Sub proves Sub equals Add in F2 (additive inverse of b is b,
// so a-b = a+b). This is the fix exercised here: the old body computed
// a+b+b = a, which is wrong.
func TestF2Sub(t *testing.T) {
  cases := []struct {
    a, b F2
    want F2
  }{
    {Zero, Zero, Zero},
    {Zero, One, One},
    {One, Zero, One},
    {One, One, Zero},
  }
  for _, c := range cases {
    if got := c.a.Sub(c.b); got != c.want {
      t.Errorf("Sub(%v, %v) = %v, want %v (Sub must equal Add in F2)", c.a, c.b, got, c.want)
    }
  }
}

// TestIsEqualStructural proves ALG-01: structural (ordered, operator+operand)
// equality. Equality is NOT commutative at the tree level — NewAdd(x,y) must
// NOT equal NewAdd(y,x).
func TestIsEqualStructural(t *testing.T) {
  x := NewVariable("x")
  y := NewVariable("y")

  // variables: name identity
  mustEqual(t, NewVariable("x"), NewVariable("x"))
  mustNotEqual(t, NewVariable("x"), NewVariable("y"))

  // constants: value identity
  mustEqual(t, NewConst(Zero), NewConstZero())
  mustNotEqual(t, NewConst(Zero), NewConst(One))
  mustEqual(t, NewConst(One), NewConstOne())

  // binary: operator + ordered operand congruence
  mustEqual(t, NewAdd(x, y), NewAdd(NewVariable("x"), NewVariable("y")))
  mustNotEqual(t, NewAdd(x, y), NewAdd(y, x)) // structural: order matters
  mustNotEqual(t, NewAdd(x, y), NewMul(x, y)) // operator matters
  mustNotEqual(t, NewAdd(x, y), NewAdd(x, NewVariable("z")))

  // symbols: name identity
  mustEqual(t, NewSymbol("M"), NewSymbol("M"))
  mustNotEqual(t, NewSymbol("M"), NewSymbol("N"))

  // cross-kind: a *NodeConst never equals a *NodeVariable even at same "value"
  mustNotEqual(t, NewConst(Zero), NewVariable("x"))
  mustNotEqual(t, NewConst(One), NewSymbol("One"))
}

// TestIsExprZeroOne proves the IsExprZero/IsExprOne predicates.
func TestIsExprZeroOne(t *testing.T) {
  if !IsExprZero(NewConstZero()) {
    t.Error("IsExprZero(NewConstZero()) = false, want true")
  }
  if IsExprZero(NewConstOne()) {
    t.Error("IsExprZero(NewConstOne()) = true, want false")
  }
  if IsExprZero(NewVariable("x")) {
    t.Error("IsExprZero(NewVariable) = true, want false")
  }

  if !IsExprOne(NewConstOne()) {
    t.Error("IsExprOne(NewConstOne()) = false, want true")
  }
  if IsExprOne(NewConstZero()) {
    t.Error("IsExprOne(NewConstZero()) = true, want false")
  }
  if IsExprOne(NewSymbol("M")) {
    t.Error("IsExprOne(NewSymbol) = true, want false")
  }
}
