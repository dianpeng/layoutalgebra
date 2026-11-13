package f2

import (
  "fmt"
  "testing"
)

// Exact-string tests for the dump facilities in dump.go. Every locked format
// choice from the plan is asserted by full-string equality, never substring
// checks, so any formatting drift fails loudly.

func TestDumpNodeString(t *testing.T) {
  a := NewVariable("a")
  b := NewVariable("b")
  c := NewVariable("c")

  cases := []struct {
    name string
    node interface{ String() string }
    want string
  }{
    {"f2 one", One, "1"},
    {"f2 zero", Zero, "0"},
    {"variable", NewVariable("t_0"), "t_0"},
    {"symbol", NewSymbol("M"), "M"},
    {"const one", NewConstOne(), "1"},
    {"const zero", NewConstZero(), "0"},
    {"xor", NewAdd(a, b), "(a ^ b)"},
    {"mul", NewMul(a, b), "(a * b)"},
    {"nested xor-mul", NewAdd(NewMul(a, b), c), "((a * b) ^ c)"},
    {"left assoc xor chain", NewAdd(NewAdd(a, b), c), "((a ^ b) ^ c)"},
  }

  for _, tc := range cases {
    if got := tc.node.String(); got != tc.want {
      t.Errorf("%s: String() = %q, want %q", tc.name, got, tc.want)
    }
  }
}

func TestDumpPrefix(t *testing.T) {
  a := NewVariable("a")
  b := NewVariable("b")
  c := NewVariable("c")

  cases := []struct {
    name string
    expr Expr
    want string
  }{
    {"xor", NewAdd(a, b), "(^ a b)"},
    {"mul", NewMul(a, b), "(* a b)"},
    {"xor of mul", NewAdd(a, NewMul(b, c)), "(^ a (* b c))"},
    {"nested left", NewAdd(NewMul(a, b), c), "(^ (* a b) c)"},
    {"leaf variable", NewVariable("t_0"), "t_0"},
    {"leaf const", NewConstOne(), "1"},
    {"leaf symbol", NewSymbol("M"), "M"},
  }

  for _, tc := range cases {
    if got := DumpPrefix(tc.expr); got != tc.want {
      t.Errorf("%s: DumpPrefix() = %q, want %q", tc.name, got, tc.want)
    }
  }
}

func TestDumpVector(t *testing.T) {
  l0 := NewVariable("l_0")
  l1 := NewVariable("l_1")
  v := NewVector([]Expr{l0, NewAdd(l0, l1), NewConstOne()})

  want := "bit 0: l_0\nbit 1: (l_0 ^ l_1)\nbit 2: 1\n"
  if got := v.Dump(); got != want {
    t.Errorf("Dump() = %q, want %q", got, want)
  }
}

func TestDumpCoeffMat(t *testing.T) {
  m := NewCoeffMat([][]Expr{
    {NewConstOne(), NewSymbol("M"), NewVariable("l_0")},
    {NewVariable("l_1"), NewConstZero(), NewConstOne()},
  })

  want := "[1 M l_0]\n[l_1 0 1]\n"
  if got := m.Dump(); got != want {
    t.Errorf("Dump() = %q, want %q", got, want)
  }
}

func TestDumpConstMat(t *testing.T) {
  m := NewConstMat([][]F2{
    {One, Zero},
    {Zero, One},
  })

  want := "[1 0]\n[0 1]\n"
  if got := m.Dump(); got != want {
    t.Errorf("Dump() = %q, want %q", got, want)
  }
}

// Proves String() is a member of the Expr interface: rendering dispatches
// through the interface value itself, with no package-level dispatcher
// involved. The Expr type annotation on tree is load-bearing.
func TestDumpExprInterfaceNative(t *testing.T) {
  a := NewVariable("a")
  b := NewVariable("b")
  c := NewVariable("c")
  var tree Expr = NewAdd(NewMul(a, b), c)
  want := "((a * b) ^ c)"

  if got := tree.String(); got != want {
    t.Errorf("Expr.String() = %q, want %q", got, want)
  }
  if got := fmt.Sprintf("%v", tree); got != want {
    t.Errorf("Sprintf(\"%%v\", expr) = %q, want %q", got, want)
  }
}

// End-to-end pipeline-stage dump: mirrors the call sequence of
// TestPipelineCodegen, so the dumped text locks the exact per-stage trees.
func TestDumpPipelineStageEndToEnd(t *testing.T) {
  l0 := NewVariable("l_0")
  l1 := NewVariable("l_1")
  l2 := NewVariable("l_2")

  hw := NewVector([]Expr{l0, l0, l2, l1})
  mapped, err := MapAccess(hw, NewVector([]Expr{l0, l0, l1, l2}))
  if err != nil {
    t.Fatalf("MapAccess error: %v", err)
  }
  strided, err := StrideMap([]int{0, 0, 0, 0}, mapped, 0)
  if err != nil {
    t.Fatalf("StrideMap error: %v", err)
  }
  final, err := SwizzleMap(Bank{Skip: 0, Range: 2, VariablePrefix: "l"}, strided)
  if err != nil {
    t.Fatalf("SwizzleMap error: %v", err)
  }

  want := "bit 0: l_0\nbit 1: (l_0 ^ l_1)\nbit 2: l_1\nbit 3: l_2\n"
  if got := final.Dump(); got != want {
    t.Errorf("pipeline stage Dump() = %q, want %q", got, want)
  }

  // coefficient-matrix stage: locked by TestPipelineCoeffMat
  cmSrc := NewVector([]Expr{l0, NewAdd(l0, l1)})
  cm, err := cmSrc.CoeffMatAll()
  if err != nil {
    t.Fatalf("CoeffMatAll error: %v", err)
  }
  wantMat := "[1 0]\n[1 1]\n"
  if got := cm.Dump(); got != wantMat {
    t.Errorf("CoeffMatAll Dump() = %q, want %q", got, wantMat)
  }
}

// Guards the no-side-effects contract: rendering must never mark or mutate.
func TestDumpReadOnly(t *testing.T) {
  a := NewVariable("a")
  b := NewVariable("b")
  c := NewVariable("c")
  tree := NewAdd(NewMul(a, b), c)
  vec := NewVector([]Expr{tree, NewConstOne()})

  _ = tree.String()
  _ = DumpPrefix(tree)
  _ = vec.Dump()

  nodes := []Expr{}
  err := Visit(
    tree,
    func(Expr) bool { return true },
    func(e Expr) error {
      nodes = append(nodes, e)
      return nil
    },
  )
  if err != nil {
    t.Fatalf("Visit error: %v", err)
  }
  for _, n := range nodes {
    if n.Marker() != 0 {
      t.Errorf("rendering set marker on %v: marker = %d, want 0", n, n.Marker())
    }
  }

  fresh := NewAdd(NewMul(NewVariable("a"), NewVariable("b")), NewVariable("c"))
  if !tree.IsEqual(fresh) {
    t.Errorf("rendering mutated tree: got %v, want IsEqual to fresh tree", tree)
  }
}
