package layout

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/internal/f2"
	"github.com/dianpeng/layoutalgebra/internal/subgroupchain"
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
	case string(SubgroupChain):
		art, err := asSgcArtifacts(m.Value)
		if err != nil {
			return nil, err
		}
		return lowerHTuple(art.layout.Coordinate)
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
	case string(SubgroupChain):
		off, err := asSgcOffset(o.Value)
		if err != nil {
			return nil, err
		}
		return lowerSGCExpr(off.expr)
	default:
		return nil, fmt.Errorf("layout: LowerOffset: unknown algebra %q", o.Algebra)
	}
}

// --------------------------------------------------------------------------------------
// F₂ lowering
// --------------------------------------------------------------------------------------

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

// --------------------------------------------------------------------------------------
// Sub-group chain lowering
// --------------------------------------------------------------------------------------

// lowerSGCExpr lowers one sub-group chain expression into the universal
// AST. The Coordinate radix extraction "(source % radix) / stride" becomes
// Div(Mod(source, radix), stride); the Stride dot-product becomes a
// left-folded Add chain of Mul(coord_i, stride_i), with the empty (rank-1)
// Stride lowering to Const 0.
func lowerSGCExpr(e subgroupchain.Expr) (ast.Expr, error) {
	switch n := e.(type) {
	case *subgroupchain.Variable:
		return ast.NewVar(n.Name(), n.Family()), nil
	case *subgroupchain.Const:
		return ast.NewConst(n.Value()), nil
	case *subgroupchain.Symbol:
		return ast.NewSymbol(n.Name()), nil
	case *subgroupchain.Binary:
		l, err := lowerSGCExpr(n.L())
		if err != nil {
			return nil, err
		}
		r, err := lowerSGCExpr(n.R())
		if err != nil {
			return nil, err
		}
		switch n.Op() {
		case subgroupchain.Add:
			return ast.NewAdd(l, r), nil
		case subgroupchain.Sub:
			return ast.NewSub(l, r), nil
		case subgroupchain.Mul:
			return ast.NewMul(l, r), nil
		case subgroupchain.Div:
			return ast.NewDiv(l, r), nil
		case subgroupchain.Mod:
			return ast.NewMod(l, r), nil
		default:
			return nil, fmt.Errorf(
				"layout: subgroupchain lowering: unknown op %d", n.Op())
		}
	case *subgroupchain.Coordinate:
		src, err := lowerSGCExpr(n.Source())
		if err != nil {
			return nil, err
		}
		return ast.NewDiv(
			ast.NewMod(src, ast.NewConst(n.Radix())),
			ast.NewConst(n.Stride()),
		), nil
	case *subgroupchain.Stride:
		strides := n.Stride()
		coords := n.Coord()
		if len(strides) != len(coords) {
			return nil, fmt.Errorf(
				"layout: subgroupchain lowering: stride/coord rank mismatch %d vs %d",
				len(strides), len(coords))
		}
		var out ast.Expr
		for i, c := range coords {
			lc, err := lowerSGCExpr(c)
			if err != nil {
				return nil, err
			}
			term := ast.NewMul(lc, ast.NewConst(strides[i]))
			if out == nil {
				out = term
			} else {
				out = ast.NewAdd(out, term)
			}
		}
		if out == nil {
			return ast.NewConst(0), nil // the rank-1 empty Stride
		}
		return out, nil
	default:
		return nil, fmt.Errorf("layout: subgroupchain lowering: unknown node %T", e)
	}
}

// lowerHTuple lowers a coordinate hierarchy into nested universal Tuples:
// leaves lower as scalar expressions, nested HTuples/Tuples recurse — one
// structural Tuple node serves both the F₂ bit vectors and the sub-group
// chain's hierarchical coordinate tuples.
func lowerHTuple(h subgroupchain.HTuple) (ast.Expr, error) {
	elem := make([]ast.Expr, 0, h.Size())
	for _, v := range h.Array() {
		var child ast.Expr
		var err error
		switch {
		case v.IsLeaf():
			child, err = lowerSGCExpr(v.Leaf())
		case v.IsTuple():
			child, err = lowerSGCTuple(v.Tuple())
		case v.IsHTuple():
			child, err = lowerHTuple(*v.HTuple())
		default:
			err = fmt.Errorf("layout: subgroupchain lowering: htuple element kind %d", v.Kind())
		}
		if err != nil {
			return nil, err
		}
		elem = append(elem, child)
	}
	return ast.NewTuple(elem), nil
}

// lowerSGCTuple lowers a native Tuple (a flat expression sequence) into a
// flat universal Tuple.
func lowerSGCTuple(t *subgroupchain.Tuple) (ast.Expr, error) {
	elem := make([]ast.Expr, 0, t.Size())
	for _, e := range t.Array() {
		child, err := lowerSGCExpr(e)
		if err != nil {
			return nil, err
		}
		elem = append(elem, child)
	}
	return ast.NewTuple(elem), nil
}
