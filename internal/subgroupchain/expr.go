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

	// ------------------------------------------------------------------------------------
	//
	// Our graph has special annotated node for representing specific information
	//
	// 1) Variable gives freedom source. We need to draw degree of freedom from a variable
	//    A variable is essentially a space of freedom provided by user. And any fine
	//    grained of freedom requires we use special expression to draw them from.
	//    Notes, a Variable is NOT a Coord node: it enters the coordinate world only
	//    as the **source** of a Coordinate, even for a full dedication.
	//
	// 2) Coordinate is a radix based digit extraction from a freedom provider:
	//
	//        (source % radix) / stride        bound = radix / stride
	//
	//    where the source is one of
	//        - Variable:   the external induction variable
	//        - Coordinate: a chained extraction (digit of a digit)
	//        - Stride:     a combined freedom pool
	//
	//    Even a trivial full dedication (the whole [0, bound) of a variable is used)
	//    goes through a Coordinate (radix == source bound, stride == 1), so every
	//    coordinate related access expression is uniformly a Coord node.
	//
	// 3) Stride performs a strided combination. Ie it always combines **MULTIPLE Coordinates**
	//    to form new extent of shape. Notes, for roubustness, Stride is also allow to
	//    do a 1:1 mapping.
	//
	// A conceptual illustration
	//
	// 1) User provided us freedom of (8, 10), denoted as variable %var0, %var1
	//
	// 2) We want to do a mapping from (8, 10) -> (2, 2, 20)
	//
	// 3.1) 2  -> Coordinate(%var1)
	// 3.2) 2  -> Coordinate(%var1)
	// 3.3) 20 -> Stride(Coordinate(%var1), Coordinate(%var2))
	//
	// In the graph, the Coordinate + Stride forms sparse coorindate induction variable to
	// exhaust the iteration space.
	//
	//
	// Key invariants:
	//
	//   - Coordinate's source must be Variable, Coordinate or Stride
	//   - Stride's operand must be a Coord node (Coordinate or Stride)
	//   - A raw Variable is never a coordinate access expression; it only appears
	//     as the source of a (possibly full-dedication) Coordinate
	//   - Coord chain monotonicity: chasing CoordChildren, UpperBound never
	//     increases — a proper digit is a strict sub-range of its source, and a
	//     Stride's rank is the product of its operands' bounds
	//
	// ------------------------------------------------------------------------------------
	KindCoordinate
	KindStride
)

type Expr interface {
	Kind() Kind
	Id() uint64

	// ------------------------------------------------------------------------------------
	//
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
	User() interface{}
	SetUser(interface{})
	IsEqual(Expr) bool
}

// --------------------------------------------------------------------------------------
//
// Any Coord node interface forms a sparse analysis chain
//
//   - Each element must be ranged
//
//   - Each element provides an induction variable that fully exhausts its [0, upperbound)
//
//   - 3 subnodes
//
//   - Variable
//
//   - Coord
//
//   - Stride
//
// (1) Coordinate -> result of factorization
// (2) Stride     -> result of combining multiple together
//
// Notes, Coordinate node, even mapping a specific portion from the dependent expression
// it is still commutative, since element in product is commutative. Which means we can
// combine different Coord node however we want, obviously with Stride node.
//
// --------------------------------------------------------------------------------------
type CoordInfo struct {
	// Partial is true for composite nodes (Coordinate/Stride), ie nodes that
	// only carry a portion of their underlying variables' freedom. A whole
	// Variable is Partial == false.
	Partial bool
	// Name is the list of variable names touched, populated by coordinate
	// analysis.
	Name []string
	// Family is the involved variables' family, populated by coordinate
	// analysis.
	Family []string

	// Shape information
	UpperBound int
}

type Coord interface {
	UpperBound() Value

	// Available after **coordinate analysis, ie sparse analysis**
	CoordInfo() *CoordInfo

	// Sparse analysis, all children points to its next coord node
	CoordChildren() []Coord
}

type coordNode struct {
	coordInfo CoordInfo
}

type exprSetup interface {
	setId(uint64)
}

type exprNode struct {
	marker int
	id     uint64
	rng    Range
	user   interface{}
}

func (self *exprNode) Id() uint64            { return self.id }
func (self *exprNode) setId(i uint64)        { self.id = i }
func (self *exprNode) Marker() int           { return self.marker }
func (self *exprNode) SetMarker(m int)       { self.marker = m }
func (self *exprNode) Range() Range          { return self.rng }
func (self *exprNode) SetRange(r Range)      { self.rng = r }
func (self *exprNode) SetUser(x interface{}) { self.user = x }
func (self *exprNode) User() interface{}     { return self.user }

// notes, this is a mutable field, we will run a simple data flow analysis algorithm to
// populate it up
func (self *coordNode) CoordInfo() *CoordInfo { return &self.coordInfo }

func (self *CoordInfo) Reset() {
	self.Partial = false
	self.Name = nil
	self.Family = nil
	self.UpperBound = 0
}

// Checking weather this coordinate access is only occupied by a single given family name
// or not
func (self *CoordInfo) IsOnlyOccupiedByFamily(familyName string) bool {
	return len(self.Family) == 1 && self.Family[0] == familyName
}

func (self *CoordInfo) HasFamily(familyName string) bool {
	for _, f := range self.Family {
		if f == familyName {
			return true
		}
	}
	return false
}

// PartiteCoord splits the coord root into 2 disjoint sets:
//
//   - lhs: Coord subtrees with NO involvement of the given familyName
//   - rhs: Coord subtrees involving ONLY the given familyName
//
// A mixed subtree (involving the family and others) is recursively split;
// a pure subtree is kept whole, which preserves each side as a single
// bijective induction variable over its combined bound. Notes the root
// itself participates: a pure root is returned whole on its side (a leaf
// Coordinate has no coord children to recurse into).
func PartiteCoord(root Coord, familyName string) ([]Coord, []Coord) {
	lhs := []Coord{}
	rhs := []Coord{}

	info := root.CoordInfo()
	has := info.HasFamily(familyName)
	only := info.IsOnlyOccupiedByFamily(familyName)
	switch {
	case !has:
		return []Coord{root}, rhs
	case only:
		return lhs, []Coord{root}
	}

	// mixed root: recurse into its children
	for _, v := range root.CoordChildren() {
		childInfo := v.CoordInfo()
		childHas := childInfo.HasFamily(familyName)
		childOnly := childInfo.IsOnlyOccupiedByFamily(familyName)
		switch {
		case !childHas:
			lhs = append(lhs, v)
		case childOnly:
			rhs = append(rhs, v)
		default: // mixed: recurse to split the subtree
			l, r := PartiteCoord(v, familyName)
			lhs = append(lhs, l...)
			rhs = append(rhs, r...)
		}
	}
	return lhs, rhs
}

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

// Variable is an external induction variable, ie a freedom provider. It is an
// Expr but deliberately NOT a Coord: coordinate accesses go through Coordinate
// nodes, with the Variable as their extraction source.
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

// Coorindate operator, ie a modular/div mapping from a some freedom provider
//
// 1) Freedom provider, ie all Coord node
//   - Variable
//   - Stride
//   - Coordinatge
//
// 2) The coordinate mapper is essentially setup as: (%var % stride) / divider
// Coordinate is a radix based digit extraction from a freedom provider:
//
//	(source % radix) / stride      with bound radix / stride
//
// The source is a Variable, another Coordinate, or a Stride (a freedom pool).
// Even a full dedication (the whole [0, bound) of the source) is expressed as
// a Coordinate with radix == source bound and stride == 1, so all coordinate
// access expressions are uniformly Coord nodes.
type Coordinate struct {
	exprNode
	coordNode
	source Expr  // freedom provider: *Variable, *Coordinate or *Stride
	radix  Value // mod mask
	stride Value // div scale
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
	coordNode
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

func (self *Stride) UpperBound() Value { return Value(self.Rank()) }

func (self *Stride) Shape() []int  { return self.shape }
func (self *Stride) Stride() []int { return self.stride }
func (self *Stride) Coord() []Expr { return self.coord }

func (self *Stride) CoordChildren() []Coord {
	o := []Coord{}
	for _, v := range self.coord {
		if c, ok := v.(Coord); ok {
			o = append(o, c)
		}
	}
	return o
}

func (self *Coordinate) Kind() Kind    { return KindCoordinate }
func (self *Coordinate) Source() Expr  { return self.source }
func (self *Coordinate) Radix() Value  { return self.radix }
func (self *Coordinate) Stride() Value { return self.stride }

// UpperBound is derived from the extraction spec (radix / stride), a single
// source of truth — there is no cached bound to drift out of sync.
func (self *Coordinate) UpperBound() Value { return self.radix / self.stride }

func (self *Coordinate) IsEqual(that Expr) bool {
	t, ok := that.(*Coordinate)
	if !ok {
		return false
	}
	return self.radix == t.radix && self.stride == t.stride && self.source.IsEqual(t.source)
}

// CoordChildren returns the coord chain under this node. The chain ends at a
// Coordinate whose source is a raw Variable (a full or partial dedication of
// that variable); the Variable itself is not a Coord node.
func (self *Coordinate) CoordChildren() []Coord {
	if c, ok := self.source.(Coord); ok {
		return []Coord{c}
	}
	return nil
}

// IsFullDedication reports whether this coordinate dedicates the entire range
// of its source (stride 1 over the full radix window).
func (self *Coordinate) IsFullDedication() bool {
	src, ok := self.source.(*Variable)
	return ok && self.UpperBound() == src.UpperBound()
}

func (self *Variable) Kind() Kind { return KindVariable }
func (self *Const) Kind() Kind    { return KindConst }
func (self *Symbol) Kind() Kind   { return KindSymbol }
func (self *Binary) Kind() Kind   { return KindBinary }

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

func (self *Var) UpperBound() Value  { return self.u }
func (self *Var) Name() string       { return self.name }
func (self *Var) Family() string     { return self.family }
func (self *Var) Same(that Var) bool { return self.name == that.name }

func (self *Variable) UpperBound() Value { return self.v.UpperBound() }
func (self *Variable) Name() string      { return self.v.Name() }
func (self *Variable) Family() string    { return self.v.Family() }
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

func (self *Symbol) UpperBound() Value { return self.v.UpperBound() }
func (self *Symbol) Name() string      { return self.v.Name() }
func (self *Symbol) Family() string    { return self.v.Family() }
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

type Replacement struct {
	Old Expr
	New Expr
}

// Replace rewrites every occurrence of r[i].Old with r[i].New inside root,
// returning the (possibly new) root. Matching is by node identity. Each
// replacement fires once, at its first match; the caller must use the return
// value, since the root itself may be replaced.
func Replace(root Expr, r []Replacement) Expr {
	queue := []*Expr{}
	queue = append(queue, &root)

	enumeratePointers := func(input Expr) []*Expr {
		out := []*Expr{}
		switch node := input.(type) {
		case *Binary:
			out = append(out, &node.l)
			out = append(out, &node.r)
		case *Stride:
			for idx := range node.coord {
				out = append(out, &node.coord[idx])
			}
		case *Coordinate:
			out = append(out, &node.source)
		}
		return out
	}

	used := make([]bool, len(r))

	for len(queue) > 0 {
		ptr := queue[0]
		queue = queue[1:]
		current := *ptr
		replaced := false
		for idx, replacement := range r {
			if used[idx] || replacement.Old == nil {
				continue
			}
			if replacement.Old == current {
				*ptr = replacement.New
				used[idx] = true
				replaced = true
				break
			}
		}

		if !replaced {
			// visiting it
			queue = append(queue, enumeratePointers(current)...)
		}
	}

	return root // notes root may be mutated as well
}

func IsCoord(x Expr) bool {
	_, ok := x.(Coord)
	return ok
}
