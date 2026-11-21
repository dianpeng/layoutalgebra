// Package ast is the universal algebra-agnostic expression tree: the compiler
// interchange form produced by every layout algebra in this module.
//
// Consumers (a code generator, a compiler backend) translate these nodes into
// their own IR; they never interpret algebra-native types. The node set is
// deliberately small and sealed — eleven kinds cover both the bit-level F₂
// algebra (Xor/And are F₂ addition/multiplication; Tuple is a bit vector,
// MSB-first) and the Z/k sub-group chain algebra (Add/Sub/Mul/Div/Mod are
// truncating integer arithmetic; Tuple is a hierarchical coordinate tuple).
//
// Nodes are immutable by convention: all fields are exported for reading only
// and there are no setters. Constructors copy any slices they are handed.
package ast

// Expr is the sealed interface implemented by every universal AST node.
// Outside implementations must not add nodes: the set is closed so lowerings
// stay total and compilers can enumerate all cases.
type Expr interface {
	isExpr()
	String() string
}

// Const is a literal integer. In F₂ context it is 0 or 1; in Z/k context it
// is any compile-time constant.
type Const struct {
	Value int
}

// Symbol is a static compile-time parameter (e.g. a tile extent M, N). It is
// substituted by the consumer at IR-lowering time — not a free variable.
type Symbol struct {
	Name string
}

// Var is a runtime variable of the hardware concurrency space. Family is
// optional semantic grouping (e.g. "warp", "lane"); F₂ bit variables carry
// no family and use the empty string.
type Var struct {
	Name   string
	Family string
}

// Xor is the F₂ addition (also: bitwise exclusive-or on integers).
type Xor struct {
	L, R Expr
}

// And is the F₂ multiplication (also: bitwise and on integers).
type And struct {
	L, R Expr
}

// Add is integer addition.
type Add struct {
	L, R Expr
}

// Sub is integer subtraction.
type Sub struct {
	L, R Expr
}

// Mul is integer multiplication.
type Mul struct {
	L, R Expr
}

// Div is truncating integer division (Go's /).
type Div struct {
	L, R Expr
}

// Mod is the remainder of truncating division (Go's %).
type Mod struct {
	L, R Expr
}

// Tuple is an ordered sequence of expressions. It serves double duty: the
// F₂ offset/bit vectors (one child per bit, MSB-first) and the sub-group
// chain's hierarchical coordinate tuples (nesting mirrors the tile shape).
// It is a structural node: Eval rejects it.
type Tuple struct {
	Elem []Expr
}

func (self *Const) isExpr()  {}
func (self *Symbol) isExpr() {}
func (self *Var) isExpr()    {}
func (self *Xor) isExpr()    {}
func (self *And) isExpr()    {}
func (self *Add) isExpr()    {}
func (self *Sub) isExpr()    {}
func (self *Mul) isExpr()    {}
func (self *Div) isExpr()    {}
func (self *Mod) isExpr()    {}
func (self *Tuple) isExpr()  {}

// NewConst returns a literal node.
func NewConst(v int) *Const { return &Const{Value: v} }

// NewSymbol returns a static compile-time parameter node.
func NewSymbol(name string) *Symbol { return &Symbol{Name: name} }

// NewVar returns a runtime variable node. Family may be empty.
func NewVar(name, family string) *Var { return &Var{Name: name, Family: family} }

// NewXor returns an F₂ addition node.
func NewXor(l, r Expr) *Xor { return &Xor{L: l, R: r} }

// NewAnd returns an F₂ multiplication node.
func NewAnd(l, r Expr) *And { return &And{L: l, R: r} }

// NewAdd returns an integer addition node.
func NewAdd(l, r Expr) *Add { return &Add{L: l, R: r} }

// NewSub returns an integer subtraction node.
func NewSub(l, r Expr) *Sub { return &Sub{L: l, R: r} }

// NewMul returns an integer multiplication node.
func NewMul(l, r Expr) *Mul { return &Mul{L: l, R: r} }

// NewDiv returns a truncating integer division node.
func NewDiv(l, r Expr) *Div { return &Div{L: l, R: r} }

// NewMod returns a Go-semantics remainder node.
func NewMod(l, r Expr) *Mod { return &Mod{L: l, R: r} }

// NewTuple returns a tuple node. The element slice is copied, so the caller
// may reuse the input slice without aliasing the node.
func NewTuple(elem []Expr) *Tuple {
	out := make([]Expr, len(elem))
	copy(out, elem)
	return &Tuple{Elem: out}
}

// Children returns the node's direct children (left-to-right), or nil for
// leaves. The returned slice must not be modified.
func Children(e Expr) []Expr {
	switch node := e.(type) {
	case *Const, *Symbol, *Var:
		return nil
	case *Xor:
		return []Expr{node.L, node.R}
	case *And:
		return []Expr{node.L, node.R}
	case *Add:
		return []Expr{node.L, node.R}
	case *Sub:
		return []Expr{node.L, node.R}
	case *Mul:
		return []Expr{node.L, node.R}
	case *Div:
		return []Expr{node.L, node.R}
	case *Mod:
		return []Expr{node.L, node.R}
	case *Tuple:
		return node.Elem
	default:
		return nil
	}
}

// Walk visits e and its subtree in pre-order. If fn returns false, that
// node's subtree is not descended into.
func Walk(e Expr, fn func(Expr) bool) {
	if e == nil || fn == nil {
		return
	}
	if !fn(e) {
		return
	}
	for _, child := range Children(e) {
		Walk(child, fn)
	}
}
