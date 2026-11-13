package f2

import (
	"fmt"
	"sort"

	"github.com/dianpeng/f2algebra/internal/check"
)

// --------------------------------------------------------------------------------------
//
// Representation of bitvector in f2algebra. A vector contains a list of expression
//
// (1) Expression is a normal F2 allowed expression, which allows 2 operators
// (2) Expression can be a constant, ie leaf nodes
//
// --------------------------------------------------------------------------------------

type Vector struct {
	vec []Expr // list of expressions
}

func (self *Vector) Size() int           { return len(self.vec) }
func (self *Vector) Rank() int           { return len(self.vec) }
func (self *Vector) Extent(idx int) Expr { return self.vec[idx] }

func (self *Vector) Gather(idxList []int) Vector {
	out := make([]Expr, len(idxList), len(idxList))
	for i, idx := range idxList {
		out[i] = self.vec[idx]
	}
	return Vector{vec: out}
}

func (self *Vector) Index(idx int) Expr { return self.vec[idx] }

// Slice returns a new Vector holding a copy of the sub-slice [start:end).
func (self *Vector) Slice(start, end int) Vector {
	out := make([]Expr, 0, end-start)
	out = append(out, self.vec[start:end]...)
	return Vector{vec: out}
}

// Set replaces the expression at idx with e.
func (self *Vector) Set(idx int, e Expr) { self.vec[idx] = e }

func (self *Vector) Fold() {
	for idx, expr := range self.vec {
		expr = Fold(expr)
		self.vec[idx] = expr
	}
}

// Add a vector to self and result in another vector
func (self *Vector) Add(that Vector) Vector {
	check.Check(
		self.Size() == that.Size(),
		fmt.Sprintf("vector add size mismatch: %d vs %d", self.Size(), that.Size()),
	)
	out := []Expr{}
	for idx, v := range that.vec {
		out = append(
			out,
			NewBinary(
				Add,
				self.vec[idx],
				v,
			),
		)
	}
	vec := Vector{vec: out}
	vec.Fold()
	return vec
}

// Multiply against a scalar in F2 types
func (self *Vector) Multiply(that F2) Vector {
	out := []Expr{}
	for _, v := range self.vec {
		out = append(
			out,
			NewBinary(
				Mul,
				v,
				NewConst(that),
			),
		)
	}
	vec := Vector{vec: out}
	vec.Fold()
	return vec
}

func (self *Vector) ShiftL(k int, overflow bool) Vector {
	vec := []Expr{}
	if overflow {
		vec = make([]Expr, len(self.vec)+k)
	} else {
		vec = make([]Expr, len(self.vec))
	}
	// vacated slots hold ConstZero, never nil Expr holes (WR-02)
	for i := range vec {
		vec[i] = NewConstZero()
	}

	for i := len(self.vec) - 1; i >= 0; i-- {
		countI := len(self.vec) - 1 - i // starting from 0 forward counter/induction variable

		// for each element reversely, always reserve K slots empty, ie K-lagging
		pos := len(vec) - 1 - k - countI
		if pos < 0 {
			break
		}
		vec[pos] = self.vec[i]
	}
	return Vector{vec: vec}
}

func (self *Vector) ShiftR(k int, overflow bool) Vector {
	vec := []Expr{}
	if overflow {
		vec = make([]Expr, len(self.vec)+k)
	} else {
		vec = make([]Expr, len(self.vec))
	}
	// vacated slots hold ConstZero, never nil Expr holes (WR-02)
	for i := range vec {
		vec[i] = NewConstZero()
	}

	for i := 0; i < len(self.vec); i++ {
		pos := k + i
		if pos >= len(vec) {
			break
		}
		vec[pos] = self.vec[i]
	}

	return Vector{vec: vec}
}

// Vector <-> Matrix based on basis provided or detected
//
// 1) Basis: this will return a list of **variables** that are touched by expression
//    tree, which can be viewed as a list of possible variables used for basis. Notes
//    we assume basis are independent free degree
//
// 2) CoeffMat: this will return the vector, decomposed, into a CoeffMat. Basically it
//    decompose each **expression** in vector as a matrix' row, with the given basis.
//    This will effectively treat all others as coefficient, ie known constants.
//    F2 guarantes we can do linear combination with ADD + MUL, ADD is variable allowed
//    MUL is scaral multiplication.
// -------------------------------------------------------------------------------------

// return a list of variables to be contained

func (self *Vector) doBasis(all []Expr) []*NodeVariable {
	dict := make(map[string]*NodeVariable)
	for _, expr := range all {
		Visit(
			expr, // root expression
			func(val Expr) bool {
				_, ok := val.(*NodeVariable)
				return ok
			},
			func(v Expr) error {
				nv := v.(*NodeVariable)
				dict[nv.Name()] = nv
				return nil
			},
		)
	}

	out := make([]*NodeVariable, 0, len(dict))
	for _, v := range dict {
		out = append(out, v)
	}
	// deterministic order: callers (CoefficientMatrix column order, BasisName)
	// rely on a stable variable sequence
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func (self *Vector) Basis() []*NodeVariable { return self.doBasis(self.vec) }
func (self *Vector) BasisName() []string {
	out := []string{}
	for _, node := range self.Basis() {
		out = append(out, node.Name())
	}
	return out
}

// GetExprVariableNameSet returns the distinct names of all *NodeVariable
// nodes in the expression tree, in deterministic (sorted) order.
func GetExprVariableNameSet(e Expr) []string {
	dict := make(map[string]bool)
	Visit(
		e,
		func(val Expr) bool {
			_, ok := val.(*NodeVariable)
			return ok
		},
		func(v Expr) error {
			dict[v.(*NodeVariable).Name()] = true
			return nil
		},
	)
	out := make([]string, 0, len(dict))
	for name := range dict {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (self *Vector) CoeffMat(basis []string) (CoeffMat, error) {
	return self.doCoeffMat(self.vec, basis)
}
func (self *Vector) CoeffMatAll() (CoeffMat, error) {
	return self.CoeffMat(self.BasisName())
}

func (self *Vector) doCoeffMat(vec []Expr, basis []string) (CoeffMat, error) {
	// slow check
	check.Check(
		func() bool {
			// ensure that all given basis shows up in the Basis list, otherwise failed
			basisList := self.Basis()
			find := func(b string) bool {
				for _, bb := range basisList {
					if bb.Name() == b {
						return true
					}
				}
				return false
			}

			for _, b := range basis {
				if !find(b) {
					return false
				}
			}
			return true
		}(),
	)

	// (1) Turning each expression tree into a tree that symbolize all unknown variables
	//     ie, treating them just as constant, but using symbol, ie unknown during'
	//     expression, interpretation
	tVec := make([]Expr, 0, self.Size())
	for _, expr := range vec {
		// Fold before extraction so duplicate same-variable closures are merged
		// into a single canonical closure (WR-01).
		tVec = append(tVec, Fold(SymbolizeExcept(expr, basis)))
	}

	// (2) Perform normalization for newly obtained vectors.
	//
	//     Effectively, for each expression in the vector, we normalize it by following
	//     1) Rank/sort the variable in a consistent order
	//     2) Ie extract its coefficient out
	//     3) Reject none linear combination with error, for example e1 * e2 is none linear
	//        Our system just cannot handle. Also affine transformation is rejected, since
	//        our system cannot handle it for now.
	//
	// ------------------------------------------------------------------------------------
	coeff := [][]Expr{}

	for _, expr := range tVec {
		tLinearCoeff, err := self.extractLinearCoeffVector(expr, basis)
		if err != nil {
			return CoeffMat{}, err // not linear typically
		}
		coeff = append(coeff, tLinearCoeff)
	}

	// (3) Create the CoeffMat accordingly
	return NewCoeffMat(coeff), nil
}

// Overall the algorithm is breaking down as following
//
// 1) greatest variable closure, which is a node in tree, s.t
//    - (1) It contains exactly one variable
//    - (2) For all sub nodes that contains exactly one variable, it has highest height
//          - Or frontier that the property (1) not held
//    - (3) Its sub expression is linear combination of involved variable
//
// 2) the root expression will be partited by set of 1) with combination nodes
//    - ie other nodes group those node in set 1) together
//
// 3) for combination node, they must be
//    - (1) NO mul, otherwise none linear
//    - (2) For add, grouping greatest variable closure with **same variables** together
//       - Groupping node is Add again
//
// 4) once we are done, we end up with a sub-expression ONLY for each specific variable
//
// 5) run Fold again for each specific sub-expression
//    - This will give us a very nice folded results, since our fold engine's main issue
//      is it cannot cross variable closure, now it has a single variable closure
//      - ie something like this will not be properly folded
//        - (2 * e1) + (2 * e2) + (3 * e1)
//        - our fold engine will be as is, no way to fold it into (5 * e1) + (2 * e1)
//        - which requires cross variable closure boundary groupping
//
// 6) Lastly extract each basis coefficient accordingly.

func (self *Vector) extractLinearCoeffVector(expr Expr, order []string) ([]Expr, error) {
	// [1] 2 passes to decide the frontier hence to get the partition knob
	//     one extra passes to assert linearity

	// [1.1] backward pass to propagate # of variables per expression in Post Order
	VisitPost(
		expr,
		func(Expr) bool { return true },
		func(e Expr) error {
			switch node := e.(type) {
			case *NodeConst:
				e.SetMarker(0)
				break

			case *NodeSymbol:
				e.SetMarker(0)
				break

			case *NodeVariable:
				e.SetMarker(1)
				break

			case *NodeBinary:
				e.SetMarker(node.L().Marker() + node.R().Marker())
				break

			default:
				break
			}
			return nil
		},
	)

	// [1.2] Check linearity, it becomes very obivious to check linearity invariants here
	//       after the # of variables information been marked.
	//
	// Linearity invariants
	//
	// (0) Denote # of variables as NOV(node): return node's number of variables
	//
	// For a given **binary node** M, NOV(node) >= 1
	//
	// (1) if NOV(L(M)) >= 1 && NOV(R(M)) == 0 && OP(M) == Add => affine
	// (2) if NOV(R(M)) >= 1 && NOV(L(M)) == 0 && OP(M) == Add => affine
	// (3) if NOV(L(M)) >= 1 && NOV(R(M)) >= 1 && OP(M) == Mul => super-linear

	if err := Visit(
		expr,
		func(b Expr) bool { return b.Kind() == Binary },
		func(e Expr) error {
			bin := e.(*NodeBinary)
			if bin.Marker() >= 1 {
				novL := bin.L().Marker()
				novR := bin.R().Marker()

				if bin.Op() == Add {
					if (novL >= 1 && novR == 0) || (novL == 0 && novR >= 1) {
						return fmt.Errorf("affine")
					}
				} else if bin.Op() == Mul {
					if novL >= 1 && novR >= 1 {
						return fmt.Errorf("super linear")
					}
				}
			}
			return nil
		},
	); err != nil {
		return nil, err
	}
	// one more case, the expression is entirely not parametrized by the basis, which is
	// yet anoter none linearity, ie affine from zero vector
	if expr.Marker() == 0 {
		if expr.Kind() != Const || !expr.(*NodeConst).Value().IsZero() {
			return nil, fmt.Errorf("affine from zero vector")
		}
	}

	// [1.3] Forward pass to get the frontier for each variable closure, hence we get
	//       the greatest variable closure. Notes this requires early exit during
	//       scanning, hence we need to use ad-hoc method
	exprMap := make(map[string]Expr)
	danglingMul := []Expr{}                                 // distributed back to expression in exprMap as multiplications
	var forwardVisit func(parent *NodeBinary, e Expr) error // need recursion
	forwardVisit = func(parent *NodeBinary, e Expr) error {
		numOfVars := e.Marker()
		if numOfVars > 1 {
			check.Check(e.Kind() == Binary)
			bin := e.(*NodeBinary)
			forwardVisit(bin, bin.L())
			forwardVisit(bin, bin.R())
			return nil
		} else if numOfVars == 1 {
			nameSet := GetExprVariableNameSet(e)
			check.Check(len(nameSet) == 1)
			name := nameSet[0]

      // In theory, we should NOT see a varaible distributed into 2 different closures
      // if we have a valid folding engine, it should handle it properly, but in case
      // we have, we need to make compensation here. This guarantee correctness in a
      // conservative way
      if existed, hasExisted := exprMap[name]; hasExisted {
        check.Check(parent != nil )
        // cross variable closure node, which by design should never happen, so be
        // conservative here
        exprMap[name] = NewBinary(parent.Op(), existed, e)
      } else {
        exprMap[name] = e
      }
		} else {
			// For this branch, we met a dangling constant term
			// 1) If <expr> add <const-term> => affine transformation, none linear reject
			// 2) If <expr> mul <const-term> => linear transformation, distributed law
			check.CheckNotNil(parent)
			check.Check(parent.Op() != Add) // none linearity, already caught must
			danglingMul = append(danglingMul, e)
		}
		return nil
	}
	if err := forwardVisit(nil, expr); err != nil {
		return nil, err
	}

	// [2] perform the per variable closure normalization
	for _, c := range danglingMul {
		check.Check(IsStaticExprRecursive(c))
		for key, val := range exprMap {
			// [2.1] distribution law
			val = NewBinary(Mul, c, val)

			// [2.2] folding
			val = Fold(val)

			// [2.3] write back
			exprMap[key] = val
		}
	}

	// [3] Now, we can extract the coefficient for our per variable closure term
	//     however complicated it is, we can be able to extract them cleanly. Basically
	//     just chasing the expression to collect all LHS symbols until the inner most
	//     RHS symbol, which is a variable (MUST) due to the fold
	out := make([]Expr, 0, len(order))
	for _, vName := range order {
		c := exprMap[vName]
		if c != nil {
			check.Check(c.Marker() == 1)
			var coeffExpr Expr // coefficient of the variable

			switch node := c.(type) {
			case *NodeBinary:
				// Variable node has its coefficient
				//
				// Our folding engine will always generate left-associative expression tree
				// hence the top level R must be a leaf node, and it must be the inner most
				// **variable**. Notes for multiple different coefficient terms, it should
				// be resolved: ie (C1 * v) + (C2 * v) => (C1 + C2) * v
				r := node.R()
				check.Check(r.Kind() == Variable)
				check.Check(r.(*NodeVariable).Name() == vName)
				coeffExpr = node.L()
				break

			case *NodeVariable:
				check.Check(node.Name() == vName)
				coeffExpr = NewConstOne()
				break

			default:
				check.Unreachable()
				break
			}
			out = append(out, coeffExpr)
		} else {
			// 0 coefficient
			out = append(out, NewConstZero())
		}
	}

	return out, nil
}

func NewWithClone(x []Expr) Vector {
	out := make([]Expr, 0, len(x))
	out = append(out, x...)
	return Vector{vec: out}
}

func NewVector(x []Expr) Vector { return Vector{vec: x} }

func NewVectorWithFold(x []Expr) Vector {
	v := NewVector(x)
	v.Fold()
	return v
}

// (1) typical setup, new a vector of bit variables from **hardware coordinates**
//
// # For example, a CUDA tile within a CTA, can be
//
// 1.1) tile on WARPs              => assume [2][2] tile
// 1.2) tile on THREADs per WARP   => assume [4][8] tile (4 * 8 = 32)
//
// vector := NewFromHardware(
//
//	[]int{1, 1, 2, 3},
//	[]string{"warp0", "warp1", "thr0", "thr1"},
//
// )
//
// Notes, the warpX/thrX's X representing the **dimension index**
func NewFromHardware(bits []int, namePrefix []string) Vector {
	check.Check(len(bits) == len(namePrefix))
	out := []Expr{}

	// each extent in bits must be power of 2
	for idx := range bits {
		bit := bits[idx]
		prefix := namePrefix[idx]
		for i := 0; i < bit; i++ {
			out = append(out, NewVariable(fmt.Sprintf("%s_%d", prefix, i)))
		}
	}

	return Vector{vec: out}
}

// --------------------------------------------------------------------------------------
//
// (2) The semantic mapping, ie the CuTE layout algebra style mapping, HTuple
//
// Assume we are talking about CUDA, the hardware enables WARP -> THREAD tile within CTA
//
// # For a (N, M) tile we can tile it per WARP then per THREAD
//
// Shape = (N, M)
// Tiled Shape = ((N0, N1), (M0, M1)) s.t N0 * N1 = N; M0 * M1 = M
//
// To avoide recursive representation of HTuple, which is overkill for us, we ask user to
// represent the tile shape in its flatten way per dimension. Notes (N0, (N1, (N2, N3)))
// is effectively (N0, N1, N2, N3).
//
// Additionally, N0, N1, N2, ... must be power of 2s
//
// # Again, user must pass in the log2(Nx) instead of Nx for simplicity
//
// The name prefix convention, similar to FromHardware, user is supposed to pass in a
// list of name prefix which will be used created variable name prefix for each dimension
// and each indexed element corresponding to that dimension's array. The prefix pattern
// is same: name<dim>_<mono_idx>
//
// Notes, the NewFromSemanticMapping can accept trivial bits, ie bit value is 0, and it
// will just ignore it automatically
//
// --------------------------------------------------------------------------------------
func NewFromSemanticMapping(dims [][]int, namePrefix []string) Vector {
	check.Check(
		len(namePrefix) >= len(dims),
		fmt.Sprintf("semantic mapping: %d dims but %d prefixes", len(dims), len(namePrefix)),
	)
	counter := make(map[string]int) // counter map for each name, notes no dimension and idx suffix
	out := []Expr{}

	for _, n := range namePrefix {
		counter[n] = 0
	}
	for dim, dimList := range dims {
		for _, v := range dimList {
			name := namePrefix[dim] // main  name
			for i := 0; i < v; i++ {
				c := counter[name] // counter
				out = append(out, NewVariable(fmt.Sprintf("%s%d_%d", name, dim, c)))
				counter[name] = c + 1
			}
		}
	}
	return Vector{vec: out}
}
