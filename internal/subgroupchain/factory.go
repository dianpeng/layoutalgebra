package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/f2algebra/internal/check"
	"github.com/dianpeng/f2algebra/internal/common"
)

// Env carries the per-analysis context: the node factory used to allocate
// expression nodes with unique ids.
type Env struct {
	Factory *Factory
}

func NewEnv() *Env {
	return &Env{Factory: &Factory{}}
}

// A simple factory to construct all types of expression nodes
type Factory struct {
	id uint64
}

func (self *Factory) nextId() uint64 {
	v := self.id
	self.id++
	return v
}

func (self *Factory) setupNode(n exprSetup) {
	n.setId(self.nextId())
}

func (self *Factory) Variable(family string, name string, upperBound int) *Variable {
	check.Check(
		upperBound >= 1,
		fmt.Sprintf("variable upper bound must be >= 1, got %d", upperBound),
	)
	out := &Variable{
		v: Var{family: family, name: name, u: upperBound},
	}
	self.setupNode(out)
	return out
}

func (self *Factory) Const(value Value) *Const {
	out := &Const{
		value: value,
	}
	self.setupNode(out)
	return out
}

func (self *Factory) Symbol(v *Variable) *Symbol {
	out := &Symbol{v: v.v}
	self.setupNode(out)
	return out
}

func (self *Factory) Binary(op Op, l, r Expr) *Binary {
	out := &Binary{
		op: op,
		l:  l,
		r:  r,
	}
	self.setupNode(out)
	return out
}

// Stride operator. The stride operator will require special setup
type StrideOffset struct {
	CoordinateExpression Expr
	UpperBound           Value
}

func (self *Factory) Stride(listOfStride []StrideOffset) *Stride {
	// figure out shape/stride/coord
	shape := make([]int, 0, len(listOfStride))
	coord := make([]Expr, 0, len(listOfStride))

	for _, v := range listOfStride {
		shape = append(shape, v.UpperBound)
		coord = append(coord, v.CoordinateExpression)
	}

	stride := common.Stride(shape)
	out := &Stride{
		shape:  shape,
		stride: stride,
		coord:  coord,
	}
	self.setupNode(out)
	return out
}

// ie PI(x0, x1, ..., xs) => x0 * x1 * ... * xs
func (self *Factory) Product(a ...Expr) Expr {
	if len(a) == 0 {
		return self.Const(1)
	}
	lhs := a[0]
	for _, v := range a[1:] {
		lhs = self.Binary(Mul, lhs, v)
	}
	return lhs
}

// Factorize performs a mixed-radix decomposition of `variable` (whose range must be
// the interval [0, PI(bounds))) into one expression per entry of `bounds`. Entry i of
// the result extracts digit i: (variable / stride[i]) % bounds[i] where
// stride[i] = PI(bounds[i+1:]). Entries of `bounds` equal to 1 yield a nil result
// entry, since that digit is trivial.
func (self *Factory) Factorize(variable Expr, bounds []int) ([]Expr, error) {
	rng := variable.Range()
	if rng == nil || rng.IsUnknown() {
		return nil, fmt.Errorf("input variable has no range attached, cannot work")
	}
	rngIt, ok := rng.(*RangeInterval)
	if !ok {
		return nil, fmt.Errorf("input variable has non-interval range, cannot work")
	}

	if len(bounds) == 0 {
		return nil, fmt.Errorf("no bounds given, cannot factorize")
	}
	for idx, b := range bounds {
		if b < 1 {
			return nil, fmt.Errorf("bounds[%d] = %d, must be >= 1", idx, b)
		}
	}

	if rngIt.UpperBound() <= 1 {
		return nil, fmt.Errorf("pointless factorization, range's upper bound is %d", rngIt.UpperBound())
	}
	if rngIt.UpperBound() != common.Product(bounds...) {
		return nil, fmt.Errorf("mismatched value, cannot factorize")
	}

	// figure out the extraction strides, with entry 0 indicating the digit is
	// trivial (bounds[idx] == 1) and must not be generated. radix[idx] is
	// PI(bounds[idx:]) and is used to mask off the higher digits before extracting
	// digit idx; it is defined even for trivial digits.
	stride := make([]int, len(bounds))
	radix := make([]int, len(bounds))
	prod := 1

	for idx := len(bounds) - 1; idx >= 0; idx-- {
		radix[idx] = prod * bounds[idx]
		if bounds[idx] == 1 {
			stride[idx] = 0
		} else {
			stride[idx] = prod
		}
		prod *= bounds[idx] // notes bounds[idx] == 1 is fine for this
	}

	// the stride contains all strided value, with entry to be 0 indicates the variable
	// is trivial, ie do not generate it directly
	out := make([]Expr, len(bounds))

	for idx, s := range stride {
		if s == 0 {
			out[idx] = nil
			continue
		}
		// current := (variable % radix[idx]) / stride[idx]
		// if idx = 0, the % radix[idx] is skipped (variable < radix[0] by
		// construction). Notes we always decompose the original variable, so no
		// running remainder state is needed.
		cur := variable
		if idx > 0 {
			cur = self.Binary(Mod, cur, self.Const(radix[idx]))
		}
		cur = self.Binary(Div, cur, self.Const(stride[idx]))
		out[idx] = cur

		// attach the range so consumers (eg the access map's next dimension) can
		// query the digit's bound without re-running analysis over the whole tree
		AnalyzeRange(cur)
	}

	return out, nil
}
