package layout

import (
	"testing"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/internal/subgroupchain"
	"github.com/dianpeng/layoutalgebra/pkg/ast"
)

// The plan's enumerated sub-group chain problems.
//
// The [6,8] SMEM problem's native pass conservatively fails (its bank
// conflicts survive the coverage repair — the swizzle stage is required),
// so a conflict-free [3,32] problem carries the equivalence/bijection
// checks while [6,8] demonstrates error propagation.

func sgcAccessProblem() layoutalgebra.Problem {
	return layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{7, 16, 4}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 14},
			{Family: "lane", Name: "t", Bound: 32},
		},
		Lane: "t",
	}
}

// sgcSmemProblem sweeps a warp over a contiguous 32-element last dimension
// of 4B elements: 32 distinct banks, conflict-free fast path.
func sgcSmemProblem() layoutalgebra.Problem {
	return layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{3, 32}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 3},
			{Family: "lane", Name: "t", Bound: 32},
		},
		Lane: "t",
	}
}

// sgcDrawProblem misorders the BoV (lane first) so the lane freedom lands in
// the major dimension — the GMEM pass actually draws family freedom into
// the last dimension instead of fast-pathing.
func sgcDrawProblem() layoutalgebra.Problem {
	return layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{2, 96}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "lane", Name: "t", Bound: 32},
			{Family: "warp", Name: "w", Bound: 6},
		},
		Lane: "t",
	}
}

func sgcSmemErrorProblem() layoutalgebra.Problem {
	return layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{6, 8}, DTypeSize: 2},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 3},
			{Family: "lane", Name: "t", Bound: 16},
		},
		Lane: "t",
	}
}

func TestRegistrySubgroupChainRegistered(t *testing.T) {
	a, ok := layoutalgebra.Lookup(string(SubgroupChain))
	if !ok {
		t.Fatalf(`Lookup("subgroupchain") not found`)
	}
	if a.Name() != string(SubgroupChain) {
		t.Errorf("Name() = %q, want %q", a.Name(), SubgroupChain)
	}
	names := layoutalgebra.Names()
	want := []string{string(F2), string(SubgroupChain)}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
			}
		}
		if !found {
			t.Errorf("Names() = %v, missing %q", names, w)
		}
	}
}

// nativeSGCAccess rebuilds the native access pipeline for p exactly per the
// adapter's conventions: BoV in the given order, flat ConstHTuple shape.
func nativeSGCAccess(t *testing.T, p *layoutalgebra.Problem) (*subgroupchain.Env, []*subgroupchain.Variable, *subgroupchain.Layout) {
	t.Helper()
	env := subgroupchain.NewEnv()
	bov := make([]*subgroupchain.Variable, 0, len(p.Concurrency))
	for _, v := range p.Concurrency {
		bov = append(bov, env.Factory.Variable(v.Family, v.Name, v.Bound))
	}
	shape := make([]subgroupchain.Value, 0, len(p.Tile.Shape))
	for _, e := range p.Tile.Shape {
		shape = append(shape, subgroupchain.Value(e))
	}
	tile := subgroupchain.NewConstHTuple(shape)
	ht, err := subgroupchain.Access(*env, bov, tile)
	if err != nil {
		t.Fatalf("native Access: %v", err)
	}
	return env, bov, &subgroupchain.Layout{Coordinate: ht, Shape: tile}
}

func nativeSGCStride(t *testing.T, env *subgroupchain.Env, layout *subgroupchain.Layout, p *layoutalgebra.Problem) subgroupchain.Expr {
	t.Helper()
	expr, err := subgroupchain.StrideMap(*env, layout, p.Tile.DTypeSize)
	if err != nil {
		t.Fatalf("native StrideMap: %v", err)
	}
	return expr
}

// enumEqual asserts AST-offset == native-offset over the full assignment
// space and that the offsets form a bijection onto the dtype multiples of
// [0, space*dtype).
func enumEqual(t *testing.T, p *layoutalgebra.Problem, offset ast.Expr, native subgroupchain.Expr, space int) {
	t.Helper()
	seen := map[int]bool{}
	count := 0
	enumerate(p, func(assign map[string]int) {
		got, err := ast.Eval(offset, assign)
		if err != nil {
			t.Fatalf("AST eval at %v: %v", assign, err)
		}
		want, err := subgroupchain.Eval(native, assign)
		if err != nil {
			t.Fatalf("native eval at %v: %v", assign, err)
		}
		if got != want {
			t.Fatalf("offset mismatch at %v: AST %d, native %d", assign, got, want)
		}
		if got%p.Tile.DTypeSize != 0 || got < 0 || got >= space*p.Tile.DTypeSize {
			t.Fatalf("offset %d is not a multiple of %d in [0,%d)",
				got, p.Tile.DTypeSize, space*p.Tile.DTypeSize)
		}
		seen[got] = true
		count++
	})
	if len(seen) != count || count != space {
		t.Errorf("offsets hit %d distinct values over %d assignments, want a bijection onto %d",
			len(seen), count, space)
	}
}

func TestSGCAccessEquivalence(t *testing.T) {
	p := sgcAccessProblem()
	eng, _ := New(SubgroupChain)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mp, err := sess.Access()
	if err != nil {
		t.Fatalf("Access: %v", err)
	}

	// the Access AST is a Tuple mirroring the flat [7,16,4] hierarchy
	tup, ok := mp.Access.(*ast.Tuple)
	if !ok {
		t.Fatalf("Access AST is %T, want *ast.Tuple", mp.Access)
	}
	if len(tup.Elem) != 3 {
		t.Errorf("Access Tuple has %d children, want 3 (one per tile dim)", len(tup.Elem))
	}

	env, _, lay := nativeSGCAccess(t, &p)
	native := nativeSGCStride(t, env, lay, &p)
	enumEqual(t, &p, mp.Offset, native, 448)
}

func TestSGCGmemEquivalence(t *testing.T) {
	p := sgcAccessProblem()
	eng, _ := New(SubgroupChain)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mp, err := sess.GmemOptimize()
	if err != nil {
		t.Fatalf("GmemOptimize: %v", err)
	}

	// native reference: GMem on a cloned layout
	env, _, lay := nativeSGCAccess(t, &p)
	clone := &subgroupchain.Layout{Coordinate: lay.Coordinate.Clone(), Shape: lay.Shape}
	err = subgroupchain.GMem(*env, subgroupchain.GMemCoalesce{
		ByteSize:       defaultLineBytes,
		VariableFamily: "lane",
	}, p.Tile.DTypeSize, clone)
	if err != nil {
		t.Fatalf("native GMem: %v", err)
	}
	enumEqual(t, &p, mp.Offset, nativeSGCStride(t, env, clone, &p), 448)
}

// The draw path: a misordered BoV forces the pass to swap family freedom
// into the last dimension — the mapping genuinely changes and must still
// agree with the native pipeline and stay a bijection.
func TestSGCGmemDrawEquivalence(t *testing.T) {
	p := sgcDrawProblem()
	eng, _ := New(SubgroupChain)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	base, err := sess.Access()
	if err != nil {
		t.Fatalf("Access: %v", err)
	}
	mp, err := sess.GmemOptimize()
	if err != nil {
		t.Fatalf("GmemOptimize: %v", err)
	}
	if base.Access.String() == mp.Access.String() {
		t.Errorf("gmem draw problem: access mapping did not change — not exercising the draw path")
	}

	env, _, lay := nativeSGCAccess(t, &p)
	clone := &subgroupchain.Layout{Coordinate: lay.Coordinate.Clone(), Shape: lay.Shape}
	err = subgroupchain.GMem(*env, subgroupchain.GMemCoalesce{
		ByteSize:       defaultLineBytes,
		VariableFamily: "lane",
	}, p.Tile.DTypeSize, clone)
	if err != nil {
		t.Fatalf("native GMem: %v", err)
	}
	enumEqual(t, &p, mp.Offset, nativeSGCStride(t, env, clone, &p), 192)
}

func TestSGCSmemEquivalence(t *testing.T) {
	p := sgcSmemProblem()
	eng, _ := New(SubgroupChain)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	mp, err := sess.SmemOptimize()
	if err != nil {
		t.Fatalf("SmemOptimize: %v", err)
	}

	env, bov, lay := nativeSGCAccess(t, &p)
	clone := &subgroupchain.Layout{Coordinate: lay.Coordinate.Clone(), Shape: lay.Shape}
	err = subgroupchain.Smem(*env, subgroupchain.SmemCoalesce{
		VariableFamily: "lane",
		BankBytes:      defaultBankBytes,
	}, bov, p.Tile.DTypeSize, clone)
	if err != nil {
		t.Fatalf("native Smem: %v", err)
	}
	enumEqual(t, &p, mp.Offset, nativeSGCStride(t, env, clone, &p), 96)
}

// The plan's [6,8] problem: the native verdict (conflicts survive the
// coverage repair, swizzle required) must propagate as an error, never a
// silently unoptimized result.
func TestSGCSmemErrorPropagation(t *testing.T) {
	p := sgcSmemErrorProblem()
	eng, _ := New(SubgroupChain)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := sess.SmemOptimize(); err == nil {
		t.Error("SmemOptimize on the [6,8] problem: want the native conservative error")
	}
}

// A lane FAMILY with two variables has no single warp sweep — the native
// validation error propagates through the facade.
func TestSGCSmemDualFamilyRejected(t *testing.T) {
	p := layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{4, 4}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "lane", Name: "a", Bound: 4},
			{Family: "lane", Name: "b", Bound: 4},
		},
		Lane: "a",
	}
	eng, _ := New(SubgroupChain)
	sess, err := eng.NewSession(p)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := sess.SmemOptimize(); err == nil {
		t.Error("SmemOptimize with a dual-variable lane family: want the unique-family error")
	}
}

// Deep-copy proof: the mutating GMem/Smem passes run on clones, so the
// registry artifacts handed to them stay untouched — lowering the original
// Mapping after optimizing still yields the original ASTs.
func TestSGCArtifactsImmutableAcrossOptimize(t *testing.T) {
	p := sgcDrawProblem()
	alg, ok := layoutalgebra.Lookup(string(SubgroupChain))
	if !ok {
		t.Fatalf("subgroupchain not registered")
	}

	m, err := alg.Access(&p)
	if err != nil {
		t.Fatalf("adapter Access: %v", err)
	}
	before, err := LowerAccess(m)
	if err != nil {
		t.Fatalf("LowerAccess: %v", err)
	}

	mg, err := alg.GmemOptimize(m, &p)
	if err != nil {
		t.Fatalf("adapter GmemOptimize: %v", err)
	}
	after, err := LowerAccess(m)
	if err != nil {
		t.Fatalf("LowerAccess after GmemOptimize: %v", err)
	}
	if before.String() != after.String() {
		t.Error("GmemOptimize mutated the input Mapping's artifacts")
	}
	optimized, err := LowerAccess(mg)
	if err != nil {
		t.Fatalf("LowerAccess(optimized): %v", err)
	}
	if optimized.String() == before.String() {
		t.Error("GmemOptimize returned the unoptimized mapping — draw path did not run")
	}

	off, err := alg.StrideMap(mg, &p)
	if err != nil {
		t.Fatalf("adapter StrideMap: %v", err)
	}
	if _, err := LowerOffset(off); err != nil {
		t.Fatalf("LowerOffset on sgc Offset: %v", err)
	}

	// same proof for the smem stage on the conflict-free problem
	sp := sgcSmemProblem()
	ms, err := alg.Access(&sp)
	if err != nil {
		t.Fatalf("adapter Access (smem problem): %v", err)
	}
	beforeS, _ := LowerAccess(ms)
	if _, err := alg.SmemOptimize(ms, &sp); err != nil {
		t.Fatalf("adapter SmemOptimize: %v", err)
	}
	afterS, _ := LowerAccess(ms)
	if beforeS.String() != afterS.String() {
		t.Error("SmemOptimize mutated the input Mapping's artifacts")
	}
}

func TestSGCUnsupportedRankMismatch(t *testing.T) {
	eng, _ := New(SubgroupChain)
	p := sgcAccessProblem()
	p.Tile.Shape = []int{7, 16, 5} // 560 != 448
	if _, err := eng.NewSession(p); err == nil {
		t.Error("NewSession with mismatched products: want error")
	}
}
