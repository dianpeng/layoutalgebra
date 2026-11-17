package subgroupchain

import (
	"github.com/dianpeng/layoutalgebra/internal/common"
)

// Symbolic expression part of subgroupchain
type Kind = int  // kind of expression nodes
type Op = int    // kind of expression opcode, notes, in the subgroupchain, we have way more operator
type Value = int // value domain, ie Z

// By default, the Z/k is a sub-group with sets of coset
//
// (1) remainder = 0  (sub-group)
// (2) remainder = 1  (translated sub-group)
// (3) remainder = 2  ...
//
// It is unlike F2, which has add + mul defined, hence form a nice linear algebra needed
// structure, vector add + scalar multiplication

const (
	Add Op = iota
	Sub
	Mul
	Div // truncating integer division, ie floor(lhs / rhs)
	Mod
)

const (
	KindConst Kind = iota
	KindSymbol
	KindVariable
	KindBinary
	KindStride
)

type Expr interface {
	Kind() Kind

	Id() uint64

	// Range analysis, each expression will have a range. Notes, the range is a simple
	// set for our framework, and it can be only 2 types, ie a single static value OR
	// a consequtive ranges.
	//
	// (1) Unknown value, this can be Const/Symbol or none normalized range
	//     - None normalized range means **range may not start from 0**
	//
	// (2) Interval value, this is used for most other parts, which means Factorize will
	//     be able to work. Notes, our range is always normalized, ie starting from 0
	//     always for now.
	//
	// ------------------------------------------------------------------------------------
	Range() Range
	SetRange(Range)

	Marker() int
	SetMarker(int)

	IsEqual(Expr) bool
}

type exprSetup interface {
	setId(uint64)
}

type exprNode struct {
	marker int
	id     uint64
	rng    Range
}

func (self *exprNode) Id() uint64       { return self.id }
func (self *exprNode) setId(i uint64)   { self.id = i }
func (self *exprNode) Marker() int      { return self.marker }
func (self *exprNode) SetMarker(m int)  { self.marker = m }
func (self *exprNode) Range() Range     { return self.rng }
func (self *exprNode) SetRange(r Range) { self.rng = r }

// Variables, ie representing external provided induction variable, each variable has its
// own domain D, by default variable ALWAYS starts at 0, hence its domain is always
// [0, U), the U is representing the upper bound, which also means a variable is always
// finite
// Notes, variable type is always integer

type Var struct {
	family string
	name   string
	u      int
}

type Variable struct {
	exprNode
	v Var
}

type Const struct {
	exprNode
	value Value
}

// symbolic constant, useful when we want to a partial analysis, ie fix a list of variables
// to be symbol and analyse the rest
type Symbol struct {
	exprNode
	v Var
}

type Binary struct {
	exprNode
	op   Op
	l, r Expr
}

// Stride operator, this is the special one that will be generated on the fly. A stride
// is a coordinate |-> 1D scalar mapping, which follows stride mapping. For example,
// 1) coordinate: c[0], c[1], ..., c[n-1]
// 2) shape:      s[0], s[1], ..., s[n-1]
// 3) stride:     ... derived by shape ...
// 4) representing the vector inner product between coordinate and stride
//
// The reason to keep this operation separate from the rest, is it simplifies the range
// analysis we need. The stride operation's range is essentially a **sub-group on Z/k**
// with a coset, these 2 parts are **not consequtive ranges**, which requires a relative
// complicated model using corresponding abstract algebra rules, feeling like a overkill
// But with stride operator, we know for sure that it is a consequtive range, ie the
// rank := PI(si) => range will be [0, rank)
//
// In our setup, the stride's shape is always **compile known**, this means we do not
// need to keep a parametrized system and drastically simplify the analysis
type Stride struct {
	exprNode
	shape  []int  // shape given
	stride []int  // cached
	coord  []Expr // this is a list of expressions
}

func (self *Stride) Kind() Kind { return KindStride }

// Rank returns PI(shape), ie the size of the exhausted 1D iteration space. An
// empty shape (extent-1 dimension) has rank 1.
func (self *Stride) Rank() int {
	if len(self.shape) == 0 {
		return 1
	}
	return common.Product(self.shape...)
}
func (self *Stride) Shape() []int  { return self.shape }
func (self *Stride) Stride() []int { return self.stride }
func (self *Stride) Coord() []Expr { return self.coord }
func (self *Variable) Kind() Kind  { return KindVariable }
func (self *Const) Kind() Kind     { return KindConst }
func (self *Symbol) Kind() Kind    { return KindSymbol }
func (self *Binary) Kind() Kind    { return KindBinary }

func (self *Stride) IsEqual(that Expr) bool {
	t, ok := that.(*Stride)
	if !ok {
		return false
	}
	if !common.SliceEqual(self.shape, t.shape) {
		return false
	}
	if !common.SliceEqual(self.stride, t.stride) {
		return false
	}
	if len(self.coord) != len(t.coord) {
		return false
	}
	for idx, rhs := range t.coord {
		if !self.coord[idx].IsEqual(rhs) {
			return false
		}
	}
	return true
}

func (self *Var) UpperBound() int    { return self.u }
func (self *Var) Name() string       { return self.name }
func (self *Var) Family() string     { return self.family }
func (self *Var) Same(that Var) bool { return self.name == that.name }

func (self *Variable) UpperBound() int { return self.v.UpperBound() }
func (self *Variable) Name() string    { return self.v.Name() }
func (self *Variable) Family() string  { return self.v.Family() }
func (self *Variable) IsEqual(that Expr) bool {
	t, ok := that.(*Variable)
	if !ok {
		return false
	}
	return self.v.Same(t.v)
}

func (self *Const) Value() Value { return self.value }
func (self *Const) IsEqual(that Expr) bool {
	t, ok := that.(*Const)
	if !ok {
		return false
	}
	return self.value == t.value
}

func (self *Symbol) UpperBound() int { return self.v.UpperBound() }
func (self *Symbol) Name() string    { return self.v.Name() }
func (self *Symbol) Family() string  { return self.v.Family() }
func (self *Symbol) IsEqual(that Expr) bool {
	t, ok := that.(*Symbol)
	if !ok {
		return false
	}
	return self.v.Same(t.v)
}

func (self *Binary) Op() Op  { return self.op }
func (self *Binary) L() Expr { return self.l }
func (self *Binary) R() Expr { return self.r }
func (self *Binary) IsEqual(that Expr) bool {
	t, ok := that.(*Binary)
	if !ok {
		return false
	}
	return self.op == t.op && self.L().IsEqual(t.L()) && self.R().IsEqual(t.R())
}
