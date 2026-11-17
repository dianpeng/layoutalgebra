package subgroupchain

import (
	"github.com/dianpeng/layoutalgebra/internal/check"
)

// HTuple, ie heirachy tuples. For each element of HTuple, it could be a leaf expression
// OR yet another Tuple.

const (
	HTLeaf   = iota // ie expression
	HTTuple         // ie a tuple expression, recursively
	HTHTuple        // ie yet another htuple expression
)

type HTupleValue struct {
	ty    int
	value interface{} // HTLeaf = Expr; HTTuple = *Tuple; HTHTuple = *HTuple
}

type ConstHTupleValue struct {
	ty    int
	value interface{} // HTLeaf = Value; HTTuple = *ConstTuple; HTHTuple = *ConstHTuple
}

type HTuple struct {
	vec []*HTupleValue
}

// All static HTuple, ie none symbolic version
type ConstHTuple struct {
	vec []*ConstHTupleValue
}

// --------------------------------------------------------------------------------------
// Constructors
// --------------------------------------------------------------------------------------

func NewHTupleValueLeaf(e Expr) *HTupleValue {
	return &HTupleValue{ty: HTLeaf, value: e}
}

func NewHTupleValueTuple(t Tuple) *HTupleValue {
	return &HTupleValue{ty: HTTuple, value: &t}
}

func NewHTupleValueHTuple(h HTuple) *HTupleValue {
	return &HTupleValue{ty: HTHTuple, value: &h}
}

// NewHTuple builds a flat HTuple whose elements are the leaves in `list`.
func NewHTuple(list []Expr) HTuple {
	vec := make([]*HTupleValue, 0, len(list))
	for _, e := range list {
		vec = append(vec, NewHTupleValueLeaf(e))
	}
	return HTuple{vec: vec}
}

func NewConstHTupleValueLeaf(v Value) *ConstHTupleValue {
	return &ConstHTupleValue{ty: HTLeaf, value: v}
}

func NewConstHTupleValueTuple(t ConstTuple) *ConstHTupleValue {
	return &ConstHTupleValue{ty: HTTuple, value: &t}
}

func NewConstHTupleValueHTuple(h ConstHTuple) *ConstHTupleValue {
	return &ConstHTupleValue{ty: HTHTuple, value: &h}
}

// NewConstHTuple builds a flat ConstHTuple whose elements are the leaves in `list`.
func NewConstHTuple(list []Value) ConstHTuple {
	vec := make([]*ConstHTupleValue, 0, len(list))
	for _, v := range list {
		vec = append(vec, NewConstHTupleValueLeaf(v))
	}
	return ConstHTuple{vec: vec}
}

// --------------------------------------------------------------------------------------
// HTupleValue
// --------------------------------------------------------------------------------------

func (self *HTupleValue) Size() int {
	switch self.ty {
	case HTLeaf:
		return 0
	case HTTuple:
		return self.value.(*Tuple).Size()
	case HTHTuple:
		return self.value.(*HTuple).Size()
	default:
		check.Unreachable()
		return -1
	}
}

func (self *HTupleValue) Rank(env Env) Expr {
	switch self.ty {
	case HTLeaf:
		return self.value.(Expr)
	case HTTuple:
		return self.value.(*Tuple).Rank(env)
	case HTHTuple:
		return self.value.(*HTuple).Rank(env)
	default:
		check.Unreachable()
		return nil
	}
}

func (self *HTupleValue) Height() int {
	switch self.ty {
	case HTLeaf:
		return 0
	case HTTuple:
		return 0
	case HTHTuple:
		return 1 + self.HTuple().Height()
	default:
		check.Unreachable()
		return -1
	}
}

func (self *HTupleValue) IsRecursive() bool {
	switch self.ty {
	case HTLeaf:
		return false
	default:
		return true
	}
}

func (self *HTupleValue) Kind() int      { return self.ty }
func (self *HTupleValue) IsLeaf() bool   { return self.Kind() == HTLeaf }
func (self *HTupleValue) IsTuple() bool  { return self.Kind() == HTTuple }
func (self *HTupleValue) IsHTuple() bool { return self.Kind() == HTHTuple }

func (self *HTupleValue) Leaf() Expr {
	check.Check(self.IsLeaf())
	return self.value.(Expr)
}

func (self *HTupleValue) Tuple() *Tuple {
	check.Check(self.IsTuple())
	return self.value.(*Tuple)
}

func (self *HTupleValue) HTuple() *HTuple {
	check.Check(self.IsHTuple())
	return self.value.(*HTuple)
}

// convert dynamically to HTuple, assert on IsRecursive
func (self *HTupleValue) AsHTuple() HTuple {
	check.Check(self.IsRecursive())
	switch self.ty {
	case HTTuple:
		return self.Tuple().ToHTuple()
	default:
		return self.HTuple().Clone()
	}
}

func (self *HTupleValue) Clone() *HTupleValue {
	out := &HTupleValue{ty: self.ty}
	switch self.ty {
	case HTLeaf:
		out.value = self.value.(Expr)
	case HTTuple:
		t := self.Tuple().Clone()
		out.value = &t
	case HTHTuple:
		h := self.HTuple().Clone()
		out.value = &h
	default:
		check.Unreachable()
	}
	return out
}

func (self *HTupleValue) SetLeaf(leaf Expr) {
	self.ty = HTLeaf
	self.value = leaf
}

func (self *HTupleValue) SetTuple(tuple Tuple) {
	self.ty = HTTuple
	self.value = &tuple
}

func (self *HTupleValue) SetHTuple(tuple HTuple) {
	self.ty = HTHTuple
	self.value = &tuple
}

// --------------------------------------------------------------------------------------
// ConstHTupleValue
// --------------------------------------------------------------------------------------

func (self *ConstHTupleValue) Size() int {
	switch self.ty {
	case HTLeaf:
		return 0
	case HTTuple:
		return self.value.(*ConstTuple).Size()
	case HTHTuple:
		return self.value.(*ConstHTuple).Size()
	default:
		check.Unreachable()
		return -1
	}
}

func (self *ConstHTupleValue) Rank() int {
	switch self.ty {
	case HTLeaf:
		return self.value.(Value)
	case HTTuple:
		return self.value.(*ConstTuple).Rank()
	case HTHTuple:
		return self.value.(*ConstHTuple).Rank()
	default:
		check.Unreachable()
		return -1
	}
}

func (self *ConstHTupleValue) Height() int {
	switch self.ty {
	case HTLeaf:
		return 0
	case HTTuple:
		return 0
	case HTHTuple:
		return 1 + self.HTuple().Height()
	default:
		check.Unreachable()
		return -1
	}
}

func (self *ConstHTupleValue) IsRecursive() bool {
	switch self.ty {
	case HTLeaf:
		return false
	default:
		return true
	}
}

func (self *ConstHTupleValue) Kind() int      { return self.ty }
func (self *ConstHTupleValue) IsLeaf() bool   { return self.Kind() == HTLeaf }
func (self *ConstHTupleValue) IsTuple() bool  { return self.Kind() == HTTuple }
func (self *ConstHTupleValue) IsHTuple() bool { return self.Kind() == HTHTuple }

func (self *ConstHTupleValue) Leaf() Value {
	check.Check(self.IsLeaf())
	return self.value.(Value)
}

func (self *ConstHTupleValue) Tuple() *ConstTuple {
	check.Check(self.IsTuple())
	return self.value.(*ConstTuple)
}

func (self *ConstHTupleValue) HTuple() *ConstHTuple {
	check.Check(self.IsHTuple())
	return self.value.(*ConstHTuple)
}

// convert dynamically to ConstHTuple, assert on IsRecursive
func (self *ConstHTupleValue) AsHTuple() ConstHTuple {
	check.Check(self.IsRecursive())
	switch self.ty {
	case HTTuple:
		return self.Tuple().ToHTuple()
	default:
		return self.HTuple().Clone()
	}
}

func (self *ConstHTupleValue) Clone() *ConstHTupleValue {
	out := &ConstHTupleValue{ty: self.ty}
	switch self.ty {
	case HTLeaf:
		out.value = self.value.(Value)
	case HTTuple:
		t := self.Tuple().Clone()
		out.value = &t
	case HTHTuple:
		h := self.HTuple().Clone()
		out.value = &h
	default:
		check.Unreachable()
	}
	return out
}

func (self *ConstHTupleValue) SetLeaf(leaf Value) {
	self.ty = HTLeaf
	self.value = leaf
}

func (self *ConstHTupleValue) SetTuple(tuple ConstTuple) {
	self.ty = HTTuple
	self.value = &tuple
}

func (self *ConstHTupleValue) SetHTuple(tuple ConstHTuple) {
	self.ty = HTHTuple
	self.value = &tuple
}

// --------------------------------------------------------------------------------------
// HTuple
// --------------------------------------------------------------------------------------

func (self *HTuple) Array() []*HTupleValue { return self.vec }

func (self *HTuple) Rank(env Env) Expr {
	if len(self.vec) == 0 {
		return env.Factory.Const(1)
	}
	exprs := make([]Expr, 0, len(self.vec))
	for _, v := range self.vec {
		exprs = append(exprs, v.Rank(env))
	}
	return env.Factory.Product(exprs...)
}

func (self *HTuple) Size() int { return len(self.vec) }

// For example: (a (b c) d) => (a b*c d)
func (self HTuple) Flatten(env Env) HTuple {
	out := make([]Expr, 0, self.Size())
	for idx := 0; idx < self.Size(); idx++ {
		v := self.vec[idx]
		out = append(out, v.Rank(env))
	}
	return NewHTuple(out)
}

// Height returns the maximum nesting depth of its elements, where top-level
// leaves sit at depth 0 and each htuple nesting adds one:
//
//	(a, (b, (c, (d, e))), f)
//	^  ^     ^  ^  ^     ^
//	0  1     2  3  3     0
func (self HTuple) Height() int {
	h := 0
	for _, v := range self.vec {
		if height := v.Height(); h < height {
			h = height
		}
	}
	return h
}

// Accessor

// 1D directional access
func (self *HTuple) Index(i int) *HTupleValue {
	check.Check(i >= 0 && i < len(self.vec))
	return self.vec[i]
}

func (self *HTuple) SetHTuple(i int, htuple HTuple) { self.vec[i].SetHTuple(htuple) }
func (self *HTuple) SetLeaf(i int, expr Expr)       { self.vec[i].SetLeaf(expr) }
func (self *HTuple) SetTuple(i int, tuple Tuple)    { self.vec[i].SetTuple(tuple) }

func (self *HTuple) Clone() HTuple {
	vec := make([]*HTupleValue, len(self.vec))
	for idx, v := range self.vec {
		vec[idx] = v.Clone()
	}
	return HTuple{vec: vec}
}

// --------------------------------------------------------------------------------------
// ConstHTuple
// --------------------------------------------------------------------------------------

func (self *ConstHTuple) Array() []*ConstHTupleValue { return self.vec }

func (self *ConstHTuple) Rank() int {
	if len(self.vec) == 0 {
		return 1
	}
	v := self.vec[0].Rank()
	for _, vv := range self.vec[1:] {
		v *= vv.Rank()
	}
	return v
}

func (self *ConstHTuple) Size() int { return len(self.vec) }

// For example: (a (b c) d) => (a b*c d)
func (self ConstHTuple) Flatten() ConstHTuple {
	out := make([]Value, 0, self.Size())
	for idx := 0; idx < self.Size(); idx++ {
		v := self.vec[idx]
		out = append(out, v.Rank())
	}
	return NewConstHTuple(out)
}

func (self ConstHTuple) ToVector() []Value {
	out := make([]Value, 0, self.Size())
	for _, v := range self.vec {
		if v.IsRecursive() {
			return nil
		}
		out = append(out, v.Leaf())
	}
	return out
}

// ToTuple converts a flat (all-leaf) ConstHTuple into a ConstTuple. It returns
// false if any element is recursive.
func (self ConstHTuple) ToTuple() (ConstTuple, bool) {
	vec := self.ToVector()
	if vec == nil && self.Size() != 0 {
		return ConstTuple{}, false
	}
	return ConstTuple{vec: vec}, true
}

// Height returns the maximum nesting depth of its elements, where top-level
// leaves sit at depth 0 and each htuple nesting adds one.
func (self *ConstHTuple) Height() int {
	h := 0
	for _, v := range self.vec {
		if height := v.Height(); h < height {
			h = height
		}
	}
	return h
}

// Accessor

// 1D directional access
func (self *ConstHTuple) Index(i int) *ConstHTupleValue {
	check.Check(i >= 0 && i < len(self.vec))
	return self.vec[i]
}

func (self *ConstHTuple) SetHTuple(i int, htuple ConstHTuple) { self.vec[i].SetHTuple(htuple) }
func (self *ConstHTuple) SetLeaf(i int, value Value)          { self.vec[i].SetLeaf(value) }
func (self *ConstHTuple) SetTuple(i int, tuple ConstTuple)    { self.vec[i].SetTuple(tuple) }

func (self *ConstHTuple) Clone() ConstHTuple {
	vec := make([]*ConstHTupleValue, len(self.vec))
	for idx, v := range self.vec {
		vec[idx] = v.Clone()
	}
	return ConstHTuple{vec: vec}
}
