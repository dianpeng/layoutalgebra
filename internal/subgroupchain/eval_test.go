package subgroupchain

import "testing"

// evalExpr is a test-only interpreter for the subgroupchain expression IR. It
// evaluates an expression tree against a concrete assignment of variable names.
func evalExpr(t *testing.T, e Expr, env map[string]int) int {
	t.Helper()
	switch n := e.(type) {
	case *Const:
		return n.Value()
	case *Variable:
		v, ok := env[n.Name()]
		if !ok {
			t.Fatalf("variable %q has no assignment", n.Name())
		}
		return v
	case *Symbol:
		v, ok := env[n.Name()]
		if !ok {
			t.Fatalf("symbol %q has no assignment", n.Name())
		}
		return v
	case *Binary:
		l := evalExpr(t, n.L(), env)
		r := evalExpr(t, n.R(), env)
		switch n.Op() {
		case Add:
			return l + r
		case Sub:
			return l - r
		case Mul:
			return l * r
		case Div:
			if r == 0 {
				t.Fatalf("invalid div %d / 0", l)
			}
			return l / r
		case Mod:
			if r == 0 {
				t.Fatalf("invalid mod %d %% 0", l)
			}
			return l % r
		default:
			t.Fatalf("unknown binary op %d", n.Op())
			return 0
		}
	case *Stride:
		total := 0
		stride := n.Stride()
		for i, c := range n.Coord() {
			total += evalExpr(t, c, env) * stride[i]
		}
		return total
	case *Coordinate:
		// a factorization digit: its value IS the underlying expression's value
		return evalExpr(t, n.Value(), env)
	default:
		t.Fatalf("unknown expression node kind %d", e.Kind())
		return 0
	}
}

// mustAccess runs Access and fails the test on error.
func mustAccess(t *testing.T, env Env, bov []*Variable, tile ConstHTuple) HTuple {
	t.Helper()
	out, err := Access(env, bov, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	return out
}

// mustStrideMap runs StrideMap and fails the test on error.
func mustStrideMap(t *testing.T, env Env, layout *Layout, dtypeSize int) Expr {
	t.Helper()
	out, err := StrideMap(env, layout, dtypeSize)
	if err != nil {
		t.Fatalf("StrideMap failed: %v", err)
	}
	return out
}
