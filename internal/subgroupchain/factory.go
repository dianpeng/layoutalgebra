package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
	"github.com/dianpeng/layoutalgebra/internal/common"
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

// NOTES(dpeng): please do not optimize case when listOfStride just contain one element
// Stride is important for various position, since many pass will rely on Stride node
// specifically
func (self *Factory) Stride(listOfStride []StrideOffset) Expr {
	allCoord := true
	for _, v := range listOfStride {
		if !self.checkCoord(v.CoordinateExpression) {
			allCoord = false
			break
		}
	}
	check.Check(allCoord, "Stride operands must be Coord nodes")

	if len(listOfStride) == 1 {
		// identity case, just return the coordinate expression back
		return listOfStride[0].CoordinateExpression
	}

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

func (self *Factory) StrideCoord(listOfCoords []Coord) Expr {
	return self.Stride(
		common.Map(listOfCoords, func(x Coord) StrideOffset {
			return StrideOffset{CoordinateExpression: x.(Expr), UpperBound: x.UpperBound()}
		}),
	)
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

// coordinate builds a radix based extraction node: (source % radix) / stride.
// The source must be a freedom provider: Variable, Coordinate or Stride.
//
// Constructor canonicalization, so the coord chain stays minimal and the
// UpperBound-monotonicity invariant holds (chasing sources, bounds never
// decrease below the extraction window):
//
//   - (R1) an extraction over the source's whole window (stride 1, radix ==
//     source bound) collapses to the source itself — eg the GMEM last-dim
//     remap yields the bare combined Stride
//   - (R2) a source Coordinate with stride 1 is a contiguous window
//     [0, radix_s) of ITS source, so (src % r)/s == (src.source % r)/s for
//     r <= radix_s — retarget through it, removing stacked windows (eg a
//     full dedication being re-factorized)
func (self *Factory) coordinate(source Expr, radix, stride Value) Expr {
	switch source.(type) {
	case *Variable, *Coordinate, *Stride:
	default:
		check.Check(
			false,
			"Coordinate source must be a Variable, Coordinate or Stride node",
		)
	}
	check.Check(
		stride >= 1,
		fmt.Sprintf("Coordinate stride must be >= 1, got %d", stride),
	)
	check.Check(
		radix >= stride && radix%stride == 0,
		fmt.Sprintf("Coordinate radix %d is not a multiple of stride %d", radix, stride),
	)
	if s, ok := source.(Coord); ok {
		check.Check(
			Value(s.UpperBound()) >= radix,
			fmt.Sprintf(
				"extraction radix %d exceeds the source window %d", radix, s.UpperBound()),
		)
	}

	// (R2) retarget through a stride-1 (contiguous window) Coordinate source
	if s, ok := source.(*Coordinate); ok && s.Stride() == 1 && radix <= s.Radix() {
		return self.coordinate(s.Source(), radix, stride)
	}
	// (R1) collapse a whole-window extraction to the source itself
	if s, ok := source.(Coord); ok && stride == 1 && Value(s.UpperBound()) == radix {
		return source
	}

	out := &Coordinate{
		source: source,
		radix:  radix,
		stride: stride,
	}
	self.setupNode(out)
	return out
}

// Dedicate wraps a whole variable as a Coordinate: the variable's full
// [0, bound) range is dedicated to the returned node (radix == bound,
// stride == 1). This is how a Variable enters the coordinate world.
func (self *Factory) Dedicate(v *Variable) *Coordinate {
	out := self.coordinate(v, v.UpperBound(), 1)
	return out.(*Coordinate)
}

func (self *Factory) checkCoord(v Expr) bool {
	_, isCoord := v.(Coord)
	return isCoord
}

// Factorize performs a mixed-radix decomposition of `variable` (whose range must be
// the interval [0, PI(bounds))) into one expression per entry of `bounds`. Entry i of
// the result extracts digit i: (variable / stride[i]) % bounds[i] where
// stride[i] = PI(bounds[i+1:]). Entries of `bounds` equal to 1 yield a nil result
// entry, since that digit is trivial.
func (self *Factory) Factorize(variable Expr, bounds []int) ([]Expr, error) {
	check.Check(self.checkCoord(variable), "Factorize input must be a Coord node")

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
		return nil, fmt.Errorf(
			"pointless factorization, range's upper bound is %d",
			rngIt.UpperBound(),
		)
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
	//
	// each digit is a Coordinate operator: (variable % radix[idx]) / stride[idx]
	// with bound radix[idx] / stride[idx] — even a whole-range digit (bounds
	// [1, ub]) is a real full-dedication Coordinate node, never the raw input
	out := make([]Expr, len(bounds))

	for idx, s := range stride {
		if s == 0 {
			out[idx] = nil
			continue
		}
		cur := self.coordinate(variable, Value(radix[idx]), Value(s))

		// attach the range so consumers (eg the access map's next dimension) can
		// query the digit's bound without re-running analysis over the whole tree
		AnalyzeRange(cur)

		out[idx] = cur
	}

	return out, nil
}

// same as Factorize, but only working for split one into 2. Assert on s0 * s1 == in.UpperBounds
func (self *Factory) Split(in Expr, s0, s1 int) (Expr, Expr, error) {
	if o, err := self.Factorize(in, []int{s0, s1}); err != nil {
		return nil, nil, err
	} else {
		return o[0], o[1], nil
	}
}
