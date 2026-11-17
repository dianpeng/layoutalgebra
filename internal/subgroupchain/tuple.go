package subgroupchain

import (
	"fmt"
)

// Tuple, similar like the f2.Vector, but more for representing the CuTE layout's tuple
// ie a flatten ordered set of value(parametrized expression)
type Tuple struct {
	vec []Expr // list of expressions
}

// tuple with everything known
type ConstTuple struct {
	vec []Value // list of consant values
}

func (self *Tuple) Size() int { return len(self.vec) }

func (self *Tuple) Array() []Expr { return self.vec }

// Rank returns the product of all elements as an expression. An empty tuple
// has rank 1 (empty product).
func (self *Tuple) Rank(env Env) Expr {
	if len(self.vec) == 0 {
		return env.Factory.Const(1)
	}
	expr := self.vec[0]
	for _, rhs := range self.vec[1:] {
		expr = env.Factory.Binary(Mul, expr, rhs)
	}
	return expr
}

func (self *Tuple) Extent(idx int) Expr { return self.vec[idx] }

func (self *Tuple) Gather(idxList []int) Tuple {
	out := make([]Expr, len(idxList))
	for i, idx := range idxList {
		out[i] = self.vec[idx]
	}
	return Tuple{vec: out}
}

func (self *Tuple) Index(idx int) Expr { return self.vec[idx] }

// Slice returns a new Tuple holding a copy of the sub-slice [start:end).
func (self *Tuple) Slice(start, end int) Tuple {
	out := make([]Expr, 0, end-start)
	out = append(out, self.vec[start:end]...)
	return Tuple{vec: out}
}

func (self *Tuple) Set(idx int, e Expr) { self.vec[idx] = e }

func (self *Tuple) Clone() Tuple {
	out := make([]Expr, len(self.vec))
	copy(out, self.vec)
	return Tuple{vec: out}
}

// ToHTuple lifts each element into a leaf of a flat HTuple.
func (self *Tuple) ToHTuple() HTuple {
	return NewHTuple(self.vec)
}

// ToConstTuple converts the tuple to a ConstTuple, failing if any element is
// not a *Const.
func (self *Tuple) ToConstTuple() (ConstTuple, error) {
	out := make([]Value, len(self.vec))
	for idx, e := range self.vec {
		c, ok := e.(*Const)
		if !ok {
			return ConstTuple{}, fmt.Errorf("tuple element %d is not a constant, cannot convert", idx)
		}
		out[idx] = c.Value()
	}
	return ConstTuple{vec: out}, nil
}

// --------------------------------------------------------------------------------------
// ConstTuple
// --------------------------------------------------------------------------------------

func (self *ConstTuple) Size() int { return len(self.vec) }

func (self *ConstTuple) Array() []Value { return self.vec }

func (self *ConstTuple) Rank() int {
	if len(self.vec) == 0 {
		return 1
	}
	v := self.vec[0]
	for _, rhs := range self.vec[1:] {
		v *= rhs
	}
	return v
}

func (self *ConstTuple) Extent(idx int) Value { return self.vec[idx] }

func (self *ConstTuple) Gather(idxList []int) ConstTuple {
	out := make([]Value, len(idxList))
	for i, idx := range idxList {
		out[i] = self.vec[idx]
	}
	return ConstTuple{vec: out}
}

func (self *ConstTuple) Index(idx int) Value { return self.vec[idx] }

// Slice returns a new ConstTuple holding a copy of the sub-slice [start:end).
func (self *ConstTuple) Slice(start, end int) ConstTuple {
	out := make([]Value, 0, end-start)
	out = append(out, self.vec[start:end]...)
	return ConstTuple{vec: out}
}

func (self *ConstTuple) Set(idx int, v Value) { self.vec[idx] = v }

func (self *ConstTuple) Clone() ConstTuple {
	out := make([]Value, len(self.vec))
	copy(out, self.vec)
	return ConstTuple{vec: out}
}

// ToHTuple lifts each element into a leaf of a flat ConstHTuple.
func (self *ConstTuple) ToHTuple() ConstHTuple {
	return NewConstHTuple(self.vec)
}
