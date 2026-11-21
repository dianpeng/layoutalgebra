package layout

import (
	"fmt"
	"testing"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/internal/f2"
	"github.com/dianpeng/layoutalgebra/pkg/ast"
)

// --------------------------------------------------------------------------------------
// Native reference evaluator (test-only): folds an F₂ expression over an
// assignment map, then packs an MSB-first vector into its integer value.
// --------------------------------------------------------------------------------------

func f2EvalNative(e f2.Expr, assign map[string]int) (int, error) {
	switch n := e.(type) {
	case *f2.NodeConst:
		return int(n.Int()), nil
	case *f2.NodeVariable:
		v, ok := assign[n.Name()]
		if !ok {
			return 0, fmt.Errorf("variable %q has no assignment", n.Name())
		}
		return v, nil
	case *f2.NodeSymbol:
		v, ok := assign[n.Name()]
		if !ok {
			return 0, fmt.Errorf("symbol %q has no assignment", n.Name())
		}
		return v, nil
	case *f2.NodeBinary:
		l, err := f2EvalNative(n.L(), assign)
		if err != nil {
			return 0, err
		}
		r, err := f2EvalNative(n.R(), assign)
		if err != nil {
			return 0, err
		}
		switch n.Op() {
		case f2.Xor:
			return l ^ r, nil
		case f2.And:
			return l & r, nil
		default:
			return 0, fmt.Errorf("native reference: op %d", n.Op())
		}
	default:
		return 0, fmt.Errorf("native reference: node %T", e)
	}
}

func f2PackNative(t *testing.T, v f2.Vector, assign map[string]int) int {
	t.Helper()
	total := 0
	for i := 0; i < v.Size(); i++ {
		b, err := f2EvalNative(v.Index(i), assign)
		if err != nil {
			t.Fatalf("native pack: %v", err)
		}
		total += b << (v.Size() - 1 - i)
	}
	return total
}

// nativeF2HW rebuilds the hardware vector exactly per the adapter's
// convention: MSB-first over Concurrency order, bits high-index-first,
// bit j named v.Name + j.
func nativeF2HW(p *layoutalgebra.Problem) f2.Vector {
	elems := []f2.Expr{}
	for _, v := range p.Concurrency {
		bits := log2Int(v.Bound)
		for j := bits - 1; j >= 0; j-- {
			elems = append(elems, f2.NewVariable(fmt.Sprintf("%s%d", v.Name, j)))
		}
	}
	return f2.NewVector(elems)
}

func nativeF2Axes(p *layoutalgebra.Problem) []f2.GmemAxis {
	axes := []f2.GmemAxis{}
	for i, extent := range p.Tile.Shape {
		if bits := log2Int(extent); bits > 0 {
			axes = append(axes, f2.GmemAxis{Name: fmt.Sprintf("d%d", i), Bits: bits})
		}
	}
	return axes
}

// --------------------------------------------------------------------------------------
// Registry
// --------------------------------------------------------------------------------------

func TestRegistryF2Registered(t *testing.T) {
	a, ok := layoutalgebra.Lookup(string(F2))
	if !ok {
		t.Fatalf(`Lookup("f2") not found — importing pkg/layout must register the adapter`)
	}
	if a.Name() != string(F2) {
		t.Errorf("Name() = %q, want %q", a.Name(), F2)
	}
	found := false
	for _, n := range layoutalgebra.Names() {
		if n == string(F2) {
			found = true
		}
	}
	if !found {
		t.Errorf("Names() = %v, missing %q", layoutalgebra.Names(), F2)
	}
}

// --------------------------------------------------------------------------------------
// Numeric equivalence: lowered AST evaluation == native pipeline evaluation
// over the FULL assignment space, per stage
// --------------------------------------------------------------------------------------

func TestF2AccessEquivalence(t *testing.T) {
	p := f2AccessProblem()
	eng, _ := New(F2)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mp, err := sess.Access()
	if err != nil {
		t.Fatalf("Access: %v", err)
	}
	if _, isTuple := mp.Access.(*ast.Tuple); !isTuple {
		t.Errorf("Access AST is %T, want *ast.Tuple", mp.Access)
	}

	// native reference pipeline
	hw := nativeF2HW(&p)
	coord, err := f2.MapAccess(hw, hw)
	if err != nil {
		t.Fatalf("native MapAccess: %v", err)
	}
	strided, err := f2.StrideMap(make([]int, coord.Size()), coord, 2)
	if err != nil {
		t.Fatalf("native StrideMap: %v", err)
	}

	count := 0
	enumerate(&p, func(assign map[string]int) {
		ba := expandBits(&p, assign)
		got, err := ast.Eval(mp.Offset, ba)
		if err != nil {
			t.Fatalf("AST eval: %v", err)
		}
		want := f2PackNative(t, strided, ba)
		if got != want {
			t.Fatalf("offset mismatch at %v: AST %d, native %d", assign, got, want)
		}
		count++
	})
	if count != 32 {
		t.Errorf("enumerated %d assignments, want 32", count)
	}
}

func TestF2GmemEquivalence(t *testing.T) {
	p := f2AccessProblem()
	eng, _ := New(F2)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mp, err := sess.GmemOptimize()
	if err != nil {
		t.Fatalf("GmemOptimize: %v", err)
	}

	hw := nativeF2HW(&p)
	res, err := f2.GmemOptimizeWith(hw, nativeF2Axes(&p), 3, 2, f2.Segment{Sector: 5, Line: 7})
	if err != nil {
		t.Fatalf("native GmemOptimizeWith: %v", err)
	}

	enumerate(&p, func(assign map[string]int) {
		ba := expandBits(&p, assign)
		got, err := ast.Eval(mp.Offset, ba)
		if err != nil {
			t.Fatalf("AST eval: %v", err)
		}
		want := f2PackNative(t, res.Offset, ba)
		if got != want {
			t.Fatalf("gmem offset mismatch at %v: AST %d, native %d", assign, got, want)
		}
	})
}

func TestF2SmemEquivalence(t *testing.T) {
	p := f2SmemProblem()
	eng, _ := New(F2)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mp, err := sess.SmemOptimize()
	if err != nil {
		t.Fatalf("SmemOptimize: %v", err)
	}

	hw := nativeF2HW(&p)
	res, err := f2.SmemOptimize(hw, hw, 2, f2.Bank{Skip: 2, Range: 5, VariablePrefix: "t"})
	if err != nil {
		t.Fatalf("native SmemOptimize: %v", err)
	}

	enumerate(&p, func(assign map[string]int) {
		ba := expandBits(&p, assign)
		got, err := ast.Eval(mp.Offset, ba)
		if err != nil {
			t.Fatalf("AST eval: %v", err)
		}
		want := f2PackNative(t, res.Offset, ba)
		if got != want {
			t.Fatalf("smem offset mismatch at %v: AST %d, native %d", assign, got, want)
		}
	})
}

// The SMEM stage propagates the native conservative error when the tile
// offers no free degree above the bank window (the 4x8 identity tiling).
func TestF2SmemErrorPropagation(t *testing.T) {
	p := f2AccessProblem()
	eng, _ := New(F2)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := sess.SmemOptimize(); err == nil {
		t.Error("SmemOptimize on the 4x8 problem: want the native no-free-degree error")
	}
}

func TestF2AccessBijection(t *testing.T) {
	p := f2AccessProblem()
	eng, _ := New(F2)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mp, err := sess.Access()
	if err != nil {
		t.Fatalf("Access: %v", err)
	}

	seen := map[int]bool{}
	enumerate(&p, func(assign map[string]int) {
		v, err := ast.Eval(mp.Offset, expandBits(&p, assign))
		if err != nil {
			t.Fatalf("AST eval: %v", err)
		}
		if v%4 != 0 || v < 0 || v >= 128 {
			t.Fatalf("offset %d is not a multiple of 4 in [0,128)", v)
		}
		seen[v] = true
	})
	if len(seen) != 32 {
		t.Errorf("access offsets hit %d distinct values, want all 32 multiples of 4 in [0,128)", len(seen))
	}
}

// Registry-path adapter chain Access -> GmemOptimize -> StrideMap must be
// numerically identical to the facade's gmem result and the native pipeline.
func TestF2AdapterChainEquivalence(t *testing.T) {
	p := f2AccessProblem()
	alg, ok := layoutalgebra.Lookup(string(F2))
	if !ok {
		t.Fatalf("f2 not registered")
	}

	m, err := alg.Access(&p)
	if err != nil {
		t.Fatalf("adapter Access: %v", err)
	}
	accessAST, err := LowerAccess(m)
	if err != nil {
		t.Fatalf("LowerAccess: %v", err)
	}
	if _, isTuple := accessAST.(*ast.Tuple); !isTuple {
		t.Errorf("LowerAccess produced %T, want *ast.Tuple", accessAST)
	}

	mg, err := alg.GmemOptimize(m, &p)
	if err != nil {
		t.Fatalf("adapter GmemOptimize: %v", err)
	}
	off, err := alg.StrideMap(mg, &p)
	if err != nil {
		t.Fatalf("adapter StrideMap: %v", err)
	}
	chainAST, err := LowerOffset(off)
	if err != nil {
		t.Fatalf("LowerOffset: %v", err)
	}

	eng, _ := New(F2)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	facade, err := sess.GmemOptimize()
	if err != nil {
		t.Fatalf("facade GmemOptimize: %v", err)
	}

	hw := nativeF2HW(&p)
	res, err := f2.GmemOptimizeWith(hw, nativeF2Axes(&p), 3, 2, f2.Segment{Sector: 5, Line: 7})
	if err != nil {
		t.Fatalf("native GmemOptimizeWith: %v", err)
	}

	enumerate(&p, func(assign map[string]int) {
		ba := expandBits(&p, assign)
		chain, err := ast.Eval(chainAST, ba)
		if err != nil {
			t.Fatalf("chain AST eval: %v", err)
		}
		facadeV, err := ast.Eval(facade.Offset, ba)
		if err != nil {
			t.Fatalf("facade AST eval: %v", err)
		}
		native := f2PackNative(t, res.Offset, ba)
		if chain != native || facadeV != native {
			t.Fatalf("adapter chain mismatch at %v: chain %d, facade %d, native %d",
				assign, chain, facadeV, native)
		}
	})
}

// LowerAccess/LowerOffset never panic on unknown algebras or wrong payload
// types — they error.
func TestLowerDispatchErrors(t *testing.T) {
	if _, err := LowerAccess(layoutalgebra.Mapping{Algebra: "nope", Value: nil}); err == nil {
		t.Error(`LowerAccess(unknown algebra): want error`)
	}
	if _, err := LowerAccess(layoutalgebra.Mapping{Algebra: string(F2), Value: 42}); err == nil {
		t.Error("LowerAccess(wrong value type): want error")
	}
	if _, err := LowerOffset(layoutalgebra.Offset{Algebra: "nope", Value: nil}); err == nil {
		t.Error(`LowerOffset(unknown algebra): want error`)
	}
	if _, err := LowerOffset(layoutalgebra.Offset{Algebra: string(F2), Value: "bad"}); err == nil {
		t.Error("LowerOffset(wrong value type): want error")
	}
}
