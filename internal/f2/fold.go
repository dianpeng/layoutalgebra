package f2

import (
	"fmt"

	"github.com/dianpeng/f2algebra/internal/check"
)

// --------------------------------------------------------------------------------------
//
// This is a very simple fold engine for normalization of the expression tree
// this is needed since during our operation, we expect a cananolical form
// Field F(2) is defined as following
//
// 1) Set/domain: {0, 1}
// 2) Op1: XOR(+)
// 3) Op2: AND(+)
//
// And both are abelian groups
//
// --------------------------------------------------------------------------------------

const folderMaxIteration = 10000

type rewriteRules interface {
	Check(Expr) bool
	Rewrite(Expr) (Expr, error)
}

type rewriteBinary struct{}

type rewriteFold struct {
	rewriteBinary
}

type rewriteCommute struct {
	rewriteBinary
}

type rewriteIdempotent struct {
	rewriteBinary
}

type rewriteReassociation struct {
	rewriteBinary
}

type rewriteDistribution struct {
	rewriteBinary
}

type rewriteNormalization struct {
	rewriteBinary
}

// symbol normalization
type rewriteSymbol struct {
	rewriteBinary
}

// rewriteRuleOrder fixes the rule iteration order. The driver MUST iterate
// rules in this exact order: Go map iteration is randomized per run, so with
// the old map-driven driver, when two rules matched the same node the chosen
// rewrite (hence the canonical form) could differ between runs. Rule structs
// are stateless (they only embed the empty rewriteBinary), so shared instances
// are safe.
var rewriteRuleOrder = []rewriteRules{
	&rewriteFold{},
	&rewriteCommute{},
	&rewriteIdempotent{},
	&rewriteReassociation{},
	&rewriteDistribution{},
	&rewriteNormalization{},
	&rewriteSymbol{},
}

func (self *rewriteBinary) Check(n Expr) bool {
	_, ok := n.(*NodeBinary)
	return ok
}

// #1: constant fold
func (self *rewriteFold) Rewrite(node Expr) (Expr, error) {
	bin := node.(*NodeBinary)
	l := bin.L()
	r := bin.R()
	// case #1: full constant, ie both are constants
	if l.Kind() == Const && r.Kind() == Const {
		lval := l.(*NodeConst).Int()
		rval := r.(*NodeConst).Int()
		if bin.Op() == Add {
			if lval != rval {
				return NewConstOne(), nil
			}
			return NewConstZero(), nil
		} else {
			if lval&rval != 0 {
				return NewConstOne(), nil
			}
			return NewConstZero(), nil
		}
	}

	// case #2: lhs is constant
	//
	// 0 + ANY = ANY
	// 1 * ANY = ANY
	// 0 * ANY = 0
	if l.Kind() == Const {
		lval := l.(*NodeConst).Value()
		if lval.IsZero() && bin.Op() == Add {
			return r, nil
		} else if lval.IsOne() && bin.Op() == Mul {
			return r, nil
		} else if lval.IsZero() && bin.Op() == Mul {
			return NewConstZero(), nil
		}
	}

	return nil, nil
}

// #2: commutative expression
//
// (1) (none_static x static)                 => (static x none_static)
// (2) ((static x none_static) x static)      => ((static x static) x none_static)
//
// --------------------------------------------------------------------------------------

func (self *rewriteCommute) Rewrite(node Expr) (Expr, error) {
	bin := node.(*NodeBinary)

	// case #1
	if !IsStaticExpr(bin.L()) && IsStaticExpr(bin.R()) {
		bin.Commute()
		return bin, nil
	}

	// case #2
	if bin.L().Kind() == Binary {
		if lNode := bin.L().(*NodeBinary); lNode.Op() == bin.Op() {
			l1 := lNode.L()
			l2 := lNode.R()
			r := bin.R()
			op := bin.Op()

			if IsStaticExpr(l1) && !IsStaticExpr(l2) && IsStaticExpr(r) {
				return NewBinary(
					op,
					NewBinary(
						op,
						l1,
						r,
					),
					l2,
				), nil
			}
		}
	}

	return nil, nil
}

// #3: idempotent
//
// 1) x + x => 0
// 2) x * x => x
func (self *rewriteIdempotent) Rewrite(node Expr) (Expr, error) {
	bin := node.(*NodeBinary)
	l := bin.L()
	r := bin.R()
	if l.IsEqual(r) {
		if bin.Op() == Add {
			return NewConstZero(), nil
		} else {
			return l, nil
		}
	}
	return nil, nil
}

// --------------------------------------------------------------------------------------
//
// reassociation of the expression, notes do not create a loop
// we always prefer left reassociation
//
// 1) convert a x (b x c)       => (a x b) x c
// 2) convert (a x b) x (c x d) => ((a x b) x c) x d
//
// x must be same operators
//
// -------------------------------------------------------------------------------------
func (self *rewriteReassociation) Rewrite(node Expr) (Expr, error) {
	bin := node.(*NodeBinary)
	binL := bin.L()
	binR := bin.R()

	// checking case 1)
	if binR.Kind() == Binary && binL.Kind() != Binary {
		if rNode := binR.(*NodeBinary); bin.Op() == rNode.Op() {
			return NewBinary(
				bin.Op(),
				NewBinary(bin.Op(), binL, rNode.L()),
				rNode.R(),
			), nil
		}
	}

	// checking case 2)
	if binL.Kind() == Binary && binR.Kind() == Binary {
		lNode := binL.(*NodeBinary)
		rNode := binR.(*NodeBinary)
		if bin.Op() == lNode.Op() && bin.Op() == rNode.Op() {
			a := lNode.L()
			b := lNode.R()
			c := rNode.L()
			d := rNode.R()
			return NewBinary(
				bin.Op(),
				NewBinary(
					bin.Op(),
					NewBinary(
						bin.Op(),
						a,
						b,
					),
					c,
				),
				d,
			), nil
		}
	}

	return nil, nil
}

// Distribution law
//
// (1) (a + b) * c => (a * c) + (b * c): This is important for later linearization

func (self *rewriteDistribution) Rewrite(node Expr) (Expr, error) {
	bin := node.(*NodeBinary)
	if bin.Op() == Mul {
		binL := bin.L()
		c := bin.R()
		if binL.Kind() == Binary {
			lNode := binL.(*NodeBinary)
			a := lNode.L()
			b := lNode.R()
			// For this case we need to rull out the **normalization** rules, since it
			// will form a cycle here. To address this, we just check to make sure a and
			// b are binary, then we fold it. Otherwise we leave it as is
			if !IsStaticExprRecursive(a) && !IsStaticExprRecursive(b) && lNode.Op() == Add {
				lhs := NewBinary(Mul, a, c)
				rhs := NewBinary(Mul, b, c)
				return NewBinary(Add, lhs, rhs), nil
			}
		}
	}

	return nil, nil
}

// Normalization
//
// (1) S1 * x + S2 * x => (S1 + S2) * x

func (self *rewriteNormalization) Rewrite(node Expr) (Expr, error) {
	bin := node.(*NodeBinary)
	if bin.Op() == Add {
		if bin.L().Kind() == Binary && bin.R().Kind() == Binary {
			binL := bin.L().(*NodeBinary)
			binR := bin.R().(*NodeBinary)
			if binL.Op() == Mul && binR.Op() == Mul {
				// a * b + c * d
				a := binL.L()
				b := binL.R()
				c := binR.L()
				d := binR.R()

				// notes we will have to use Recursive for ad-hoc normalization here. Basically
				// any constant combination of a expression trees will lead to constancy. A
				// better approach is to perform a data flow analysis to assert expression's
				// constantcy, hence we can sure that we just reassociate it directly.
				if IsStaticExprRecursive(a) && IsStaticExprRecursive(c) && b.IsEqual(d) {
					coeff := NewBinary(Add, a, c)
					return NewBinary(Mul, coeff, b), nil
				}
			}
		}

		// (2) v + c * v => (1 + c) * v — bare-variable coefficient-1 merge.
		// rewriteCommute does not normalize this order (a Mul is not fully
		// static), so each operand order needs its own branch. The Mul side must
		// carry a static coefficient over the SAME variable; anything else
		// (different variable, non-static coefficient) stays untouched.
		if bin.L().Kind() == Variable {
			if rNode, ok := bin.R().(*NodeBinary); ok && rNode.Op() == Mul {
				if IsStaticExprRecursive(rNode.L()) && rNode.R().IsEqual(bin.L()) {
					coeff := NewBinary(Add, NewConstOne(), rNode.L())
					return NewBinary(Mul, coeff, bin.L()), nil
				}
			}
		}

		// (3) c * v + v => (c + 1) * v — symmetric bare-variable merge.
		if bin.R().Kind() == Variable {
			if lNode, ok := bin.L().(*NodeBinary); ok && lNode.Op() == Mul {
				if IsStaticExprRecursive(lNode.L()) && lNode.R().IsEqual(bin.R()) {
					coeff := NewBinary(Add, lNode.L(), NewConstOne())
					return NewBinary(Mul, coeff, bin.R()), nil
				}
			}
		}
	}
	return nil, nil
}

// Symbol
//
// We need to normalize symbol as well, since the matrix operation will perform symbolic
// analysis instead of targeting on constant only. This means we need to have a even
// finer consistency order within symbol and also symbol against const
//
// 1) For order between symbol and const: const => lhs and symbol => rhs
// 2) For symbol against symbols: order them lexical graphically
//
// This forms a cananolical representation of coefficient of any expressions. This is
// super important when testing equalvelence. Our equal function DOES not compare actual
// congurence relationship, we sololy rely on the representational equavlence
//
// It is possible to implement something like e-graph to proove conguruence, we may add
// it in the future.
func (self *rewriteSymbol) Rewrite(node Expr) (Expr, error) {
	bin := node.(*NodeBinary)
	l := bin.L()
	r := bin.R()

	// 1) push const to lhs and symbol to rhs
	if IsStaticExpr(l) && IsStaticExpr(r) {
		if l.Kind() == Symbol && r.Kind() == Const {
			bin.Commute()
		}
	}

	// lexical order of symbols
	//
	// (1): (sym1 sym2)
	// (2): ((const sym1) sym2)

	// case #1
	if l.Kind() == Symbol && r.Kind() == Symbol {
		lSym := l.(*NodeSymbol)
		rSym := r.(*NodeSymbol)
		if lSym.Name() > rSym.Name() {
			bin.Commute()
			return bin, nil
		}
	}

	// case #2
	if l.Kind() == Binary && r.Kind() == Symbol {
		lNode := l.(*NodeBinary)
		l0 := lNode.L()
		l1 := lNode.R()
		r := bin.R()
		if bin.Op() == lNode.Op() && l0.Kind() == Const && l1.Kind() == Symbol && r.Kind() == Symbol {
			l1Sym := l1.(*NodeSymbol)
			rSym := r.(*NodeSymbol)
			if l1Sym.Name() > rSym.Name() {
				return NewBinary(
					bin.Op(),
					NewBinary(
						lNode.Op(),
						l0,
						r,
					),
					l1,
				), nil
			}
		}
	}

	return nil, nil
}

// -------------------------------------------------------------------------------------
// Driver: classical run-to-fixed-point
// -------------------------------------------------------------------------------------

type folder struct {
	maxBound int
	current  []Replacement
}

func (self *folder) rewrite(expr Expr) error {
	didReplace := false
	for _, r := range rewriteRuleOrder {
		if r.Check(expr) {
			replacement, err := r.Rewrite(expr)
			if err != nil {
				return err
			}
			if replacement != nil {
				self.current = append(self.current, Replacement{
					Old: expr,
					New: replacement,
				})
				didReplace = true
				break
			}
		}
	}

	if !didReplace {
		// visiting the expression recursively if not replaced
		switch n := expr.(type) {
		case *NodeBinary:
			if err := self.rewrite(n.L()); err != nil {
				return err
			}
			if err := self.rewrite(n.R()); err != nil {
				return err
			}
			break
		default:
			break
		}
	}
	return nil
}

func (self *folder) doRun(expr Expr) ([]Replacement, error) {
	self.current = nil // reset current replacement queue
	if err := self.rewrite(expr); err != nil {
		return nil, err // rewrite rule errors must not be swallowed (WR-10a)
	}
	return self.current, nil
}

func (self *folder) run(expr Expr) Expr {
	var replacementQueue []Replacement
	var err error
	for i := 0; i < self.maxBound; i++ {
		replacementQueue, err = self.doRun(expr)
		if err != nil {
			check.Bug(fmt.Sprintf("fold: rewrite error: %v", err))
		}
		if len(replacementQueue) == 0 {
			break
		}
		expr = Replace(expr, replacementQueue)
	}
	// A non-empty queue after the loop means the cap was exhausted before
	// reaching a fixpoint — the result would be a silent non-canonical
	// expression, which defeats the engine's entire purpose (WR-10b)
	if len(replacementQueue) != 0 {
		check.Bug(fmt.Sprintf("fold: iteration cap %d exhausted before canonical form", self.maxBound))
	}
	return expr
}

func Fold(expr Expr) Expr {
	f := &folder{maxBound: folderMaxIteration}
	return f.run(expr)
}
