package f2

import "fmt"

// Expression is always a tree, never even a DAG for now. For now, the expression level
// simplification never generates **expression elimination**, which means the input
// expression tree will remain as is structurally. This is important for guarantee of
// correctness and simpliness. Implicitly, it also means each node is unique in the
// tree, which is also very critical for us.

type F2 = bool // scalar type

type NodeKind = int

const (
  Zero F2 = false
  One F2 = true
)

const (
  Variable NodeKind = iota
  Const
  Binary
  Symbol // this is also constant, but ie a symbol based constant, not literals
)

const (
  And = iota
  Xor

  // F(2) defined operators for Mul/Add
  Mul = And
  Add = Xor
)

type Expr interface {
  Kind() NodeKind

  // Congurence
  //
  // (1) variable's congurence is defined by name identical
  // (2) constant's congurence is defined by value identical
  // (3) expresison's concgurence is defined by operator identical and all operands to
  //     be congurent
  IsEqual(Expr) bool

  SetMarker(int)
  Marker() int
}

type Static interface {
  Expr
  IsSymbol() bool
  IsConst() bool
  staticNode()
}

type nodeMarker struct {
  marker int
}

func (self *nodeMarker) SetMarker(v int) { self.marker = v }
func (self *nodeMarker) Marker() int { return self.marker }

type NodeVariable struct {
  nodeMarker
  name string // human readable name of the node
}

type NodeConst struct {
  nodeMarker
  value F2 // 0/1
}

type NodeBinary struct {
  nodeMarker
  op int
  l Expr
  r Expr
}

type NodeSymbol struct {
  nodeMarker
  name string
}

type Replacement struct {
  Old Expr
  New Expr
}

func Replace(root Expr, r []Replacement) Expr {
  queue := []*Expr{}
  queue = append(queue, &root)

  enumeratePointers := func(input Expr) []*Expr {
    out := []*Expr{}
    if tB, ok := input.(*NodeBinary); ok {
      out = append(out, &tB.l)
      out = append(out, &tB.r)
    }
    return out
  }

  for len(queue) > 0 {
    ptr := queue[0]
    queue = queue[1:]
    current := *ptr
    replaced := false
    for _, replacement := range r {
      if replacement.Old == nil {
        continue
      }
      if replacement.Old == current {
        *ptr = replacement.New
        replacement.Old = nil
        replaced = true
      }
    }

    if !replaced {
      // visiting it
      queue = append(queue, enumeratePointers(current)...)
    }
  }

  return root // notes root may be mutated as well
}

func (self F2) IsZero() bool { return !self }
func (self F2) IsOne() bool  { return self }
func (self F2) Int() uint {
  if self {
    return 1
  }
  return 0
}

func (self *NodeVariable) Kind() NodeKind { return Variable }
func (self *NodeVariable) Name() string {return self.name }
func (self *NodeVariable) IsEqual(that Expr) bool {
  if tN, ok := that.(*NodeVariable); ok {
    return tN.name == self.name
  }
  return false
}

func (self *NodeConst) Int() uint {
  if self.value {
    return 1
  }
  return 0
}
func (self *NodeConst) Value() F2 { return self.value }
func (self *NodeConst) Kind() NodeKind { return Const }
func (self *NodeConst) IsEqual(that Expr) bool {
  if tN, ok := that.(*NodeConst); ok {
    return tN.value == self.value
  }
  return false
}
func (self *NodeConst) staticNode() {}
func (self *NodeConst) IsConst() bool { return true }
func (self *NodeConst) IsSymbol() bool { return false }

func (self *NodeBinary) Kind() NodeKind { return Binary }
func (self *NodeBinary) IsEqual(that Expr) bool {
  if tN, ok := that.(*NodeBinary); ok {
    if tN.op == self.op {
      return self.l.IsEqual(that.l) && self.r.IsEqual(that.r)
    }
  }
  return false
}
func (self *NodeBinary) Op() int { return self.op }
func (self *NodeBinary) L() Expr { return self.l }
func (self *NodeBinary) R() Expr { return self.r }
func (self *NodeBinary) Commute() {
  l := self.l
  self.l = self.r
  self.r = l
}

func (self *NodeSymbol) Kind() NodeKind { return Symbol }
func (self *NodeSymbol) Name() string { return self.name }
func (self *NodeSymbol) IsEqual(that Expr) bool {
  if tN, ok := that.(*NodeSymbol); ok {
    return tN.name == self.name
  }
  return false
}
func (self *NodeSymbol) staticNode() {}
func (self *NodeSymbol) IsConst() bool { return false }
func (self *NodeSymbol) IsSymbol() bool { return true }

func (self F2) Add(that F2) F2 { return self != that }
func (self F2) Sub(that F2) F2 { return self.Add(that).Add(that) }
func (self F2) Mul(that F2) F2 { return self && that }

// Factory method
func NewConst(value F2) *NodeConst { return &NodeConst{value: value} }
func NewConstZero() *NodeConst { return NewConst(Zero) }
func NewConstOne() *NodeConst { return NewConst(One) }
func NewBinary(op int, l, r Expr) *NodeBinary { return &NodeBinary{op: op, l: l, r: r} }
func NewAdd(l, r Expr) *NodeBinary { return NewBinary(Add, l, r) }
func NewMul(l, r Expr) *NodeBinary { return NewBinary(Mul, l, r) }
func NewSymbol(s string) *NodeSymbol { return &NodeSymbol{name: s} }
func NewVariable(s string) *NodeVariable { return &NodeVariable{name: s} }

func IsLeafExpr(e Expr) bool { return e.Kind() != Binary }
func IsInternalExpr(e Expr) bool { return !IsLeafExpr(e) }
func IsStaticExpr(e Expr) bool { return e.Kind() == Const || e.Kind() == Symbol }

// This is a data flow analysis to assert an expression to be fully static, ie all
// operands involves are symbol/const
func IsStaticExprRecursive(e Expr) bool {
  err := VisitPost(
    e,
    func (e Expr) bool { return true },
    func (e Expr) error {
      switch e.(type) {
        case *NodeConst, *NodeSymbol:
          return nil
        case *NodeVariable:
          return fmt.Errorf("invalid")
        case *NodeBinary:
          // Post order, hence operands are verified already
          return nil
      }
      return nil
    },
  )
  return err == nil
}

func IsExprZero(e Expr) bool { return e.Kind() == Const && e.(*NodeConst).Value().IsZero() }
func IsExprOne(e Expr) bool { return e.Kind() == Const && e.(*NodeConst).Value().IsOne() }

// Symbolize, this is typically used as an intermediate tree construction, s.t to put
// a set of Variables into a set of Symbols. Symbol representing constants that we are
// not known during the runtime analysis. This is correpsonding to lots of automatic
// prover/SAT sovler/LP solver's symbolic parameters. Constant will be parametrized with
// symbolic variables
func doSymbolize(
  root Expr,
  judge func(*NodeVariable) bool,
) Expr {
   rep := []Replacement{}
   Visit(
    root,
    func(e Expr) bool { return e.Kind() == Variable },
    func(e Expr) error {
      v := e.(*NodeVariable)
      if judge(v) {
        rep = append(rep, Replacement{
          Old: e,
          New: NewSymbol(v.Name()),
        })
      }
      return nil
    },
  )
   return Replace(root, rep)
}

func Symbolize(
  root Expr,
  names []string,
) Expr {
  return doSymbolize(root, func(n *NodeVariable) bool {
    for _, v := range names {
      if v == n.Name() {
        return true
      }
    }
    return false
  })
}

func SymbolizeExcept(
  root Expr,
  names []string,
) Expr {
  return doSymbolize(root, func(n *NodeVariable) bool {
    for _, v := range names {
      if v == n.Name() {
        return false
      }
    }
    return true
  })
}
