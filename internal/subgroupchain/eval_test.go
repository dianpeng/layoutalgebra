package subgroupchain

import "testing"

// evalExpr is a test helper delegating to the production Eval, failing the
// test on error.
func evalExpr(t *testing.T, e Expr, env map[string]int) int {
	t.Helper()
	v, err := Eval(e, env)
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}
	return v
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
