package layout

import (
	"errors"
	"fmt"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/internal/f2"
	"github.com/dianpeng/layoutalgebra/pkg/ast"
)

// LowerAccess lowers a registry-path Mapping (produced by the f2 /
// subgroupchain Algebra adapters) into the universal AST: the
// tile-coordinate mapping, as a Tuple. It errors on unknown algebra names
// and wrong Value dynamic types — it never panics on an opaque payload.
func LowerAccess(m layoutalgebra.Mapping) (ast.Expr, error) {
	switch m.Algebra {
	case string(F2):
		art, err := asF2Artifacts(m.Value)
		if err != nil {
			return nil, err
		}
		return lowerF2Vector(art.coord)
	default:
		return nil, fmt.Errorf("layout: LowerAccess: unknown algebra %q", m.Algebra)
	}
}

// LowerOffset lowers a registry-path Offset into the universal AST: the
// final byte-offset expression the compiler translates into its own IR.
// Same conservative dispatch contract as LowerAccess.
func LowerOffset(o layoutalgebra.Offset) (ast.Expr, error) {
	switch o.Algebra {
	case string(F2):
		off, err := asF2Offset(o.Value)
		if err != nil {
			return nil, err
		}
		return lowerF2Offset(off.vec)
	default:
		return nil, fmt.Errorf("layout: LowerOffset: unknown algebra %q", o.Algebra)
	}
}

// lowerF2Expr lowers one F₂ expression into the universal AST. F₂ operators
// map to the bit-level nodes: Xor is F₂ addition, And is F₂ multiplication.
func lowerF2Expr(e f2.Expr) (ast.Expr, error) {
	switch n := e.(type) {
	case *f2.NodeVariable:
		return ast.NewVar(n.Name(), ""), nil
	case *f2.NodeConst:
		return ast.NewConst(int(n.Int())), nil
	case *f2.NodeSymbol:
		return ast.NewSymbol(n.Name()), nil
	case *f2.NodeBinary:
		l, err := lowerF2Expr(n.L())
		if err != nil {
			return nil, err
		}
		r, err := lowerF2Expr(n.R())
		if err != nil {
			return nil, err
		}
		switch n.Op() {
		case f2.Xor:
			return ast.NewXor(l, r), nil
		case f2.And:
			return ast.NewAnd(l, r), nil
		default:
			return nil, fmt.Errorf(
				"layout: f2 lowering: op %d is not an F2 operator", n.Op())
		}
	default:
		return nil, fmt.Errorf("layout: f2 lowering: unknown node %T", e)
	}
}

// lowerF2Vector lowers an MSB-first F₂ vector into a Tuple with one child
// per bit, MSB-first.
func lowerF2Vector(v f2.Vector) (ast.Expr, error) {
	elem := make([]ast.Expr, 0, v.Size())
	for i := 0; i < v.Size(); i++ {
		child, err := lowerF2Expr(v.Index(i))
		if err != nil {
			return nil, err
		}
		elem = append(elem, child)
	}
	return ast.NewTuple(elem), nil
}

// lowerF2Offset lowers an MSB-first byte-space bit vector into the weighted
// sum Σ bit_i · 2^(n-1-i) — an Add/Mul chain over distinct powers of two,
// so the bits never carry. An empty vector lowers to Const 0.
func lowerF2Offset(v f2.Vector) (ast.Expr, error) {
	n := v.Size()
	if n == 0 {
		return ast.NewConst(0), nil
	}
	var out ast.Expr
	for i := 0; i < n; i++ {
		bit, err := lowerF2Expr(v.Index(i))
		if err != nil {
			return nil, err
		}
		term := ast.NewMul(bit, ast.NewConst(1<<(n-1-i)))
		if out == nil {
			out = term
		} else {
			out = ast.NewAdd(out, term)
		}
	}
	return out, nil
}

// sgcBackend stub — replaced by subgroupchain.go in the next task.
type sgcBackend struct{}

func (sgcBackend) kind() Kind { return SubgroupChain }

func (sgcBackend) supported(*layoutalgebra.Problem) bool { return false }

func (sgcBackend) access(*layoutalgebra.Problem, options) (*Mapping, error) {
	return nil, errors.New("not implemented")
}

func (sgcBackend) gmem(*layoutalgebra.Problem, options) (*Mapping, error) {
	return nil, errors.New("not implemented")
}

func (sgcBackend) smem(*layoutalgebra.Problem, options) (*Mapping, error) {
	return nil, errors.New("not implemented")
}
