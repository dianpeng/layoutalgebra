package layout

import (
	"fmt"
	"testing"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/pkg/ast"
)

// Shared fixtures and enumeration helpers for the per-algebra test files.

// f2AccessProblem is the plan's enumerated F₂ access problem: 4x8 tile of
// 4B elements, 4 warps x 8 lanes = 32 assignments.
func f2AccessProblem() layoutalgebra.Problem {
	return layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{4, 8}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 4},
			{Family: "lane", Name: "t", Bound: 8},
		},
		Lane: "t",
	}
}

// f2SmemProblem is sized so the bank window (2, 5) sits entirely inside the
// lane-bit run of the strided vector: the identity-tiling SMEM pipeline
// saturates cleanly instead of running out of free degree.
func f2SmemProblem() layoutalgebra.Problem {
	return layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{4, 32}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 2},
			{Family: "lane", Name: "t", Bound: 64},
		},
		Lane: "t",
	}
}

// enumerate walks the full concurrency space of p in declaration order,
// invoking fn with a fresh variable assignment for every point.
func enumerate(p *layoutalgebra.Problem, fn func(map[string]int)) {
	assign := map[string]int{}
	var rec func(int)
	rec = func(i int) {
		if i == len(p.Concurrency) {
			copied := make(map[string]int, len(assign))
			for k, v := range assign {
				copied[k] = v
			}
			fn(copied)
			return
		}
		v := p.Concurrency[i]
		for x := 0; x < v.Bound; x++ {
			assign[v.Name] = x
			rec(i + 1)
		}
	}
	rec(0)
}

// expandBits extends a variable-level assignment with the per-bit
// assignments the F₂ algebra's lowered ASTs consume: bit j of variable v is
// named v.Name + j. Variable-level names are kept too (the sub-group chain
// ASTs consume those directly).
func expandBits(p *layoutalgebra.Problem, assign map[string]int) map[string]int {
	out := make(map[string]int, len(assign)+8)
	for k, v := range assign {
		out[k] = v
	}
	for _, v := range p.Concurrency {
		bits, err := log2Checked(v.Bound)
		if err != nil {
			panic(fmt.Sprintf("expandBits: %v", err))
		}
		for j := 0; j < bits; j++ {
			out[fmt.Sprintf("%s%d", v.Name, j)] = (assign[v.Name] >> j) & 1
		}
	}
	return out
}

func TestNewRejectsUnknownKind(t *testing.T) {
	if _, err := New(Kind("bogus")); err == nil {
		t.Error(`New(Kind("bogus")): want error`)
	}
}

func TestNewF2Succeeds(t *testing.T) {
	eng, err := New(F2)
	if err != nil {
		t.Fatalf("New(F2): %v", err)
	}
	if eng.Kind() != F2 {
		t.Errorf("Kind() = %q, want %q", eng.Kind(), F2)
	}
}

func TestOptionValidation(t *testing.T) {
	bad := []struct {
		name string
		opt  Option
	}{
		{"line bytes not pow2", WithLineBytes(96)},
		{"line bytes too small", WithLineBytes(16)},
		{"bank bytes zero", WithBankBytes(0)},
		{"bank skip negative", WithBankWindow(-1, 5)},
		{"bank range zero", WithBankWindow(2, 0)},
	}
	for _, c := range bad {
		if _, err := New(F2, c.opt); err == nil {
			t.Errorf("%s: New accepted the option", c.name)
		}
	}
	if _, err := New(F2, WithLineBytes(64), WithBankBytes(256), WithBankWindow(2, 5)); err != nil {
		t.Errorf("valid options rejected: %v", err)
	}
}

func TestNewSessionRejectsInvalidProblems(t *testing.T) {
	eng, err := New(F2)
	if err != nil {
		t.Fatalf("New(F2): %v", err)
	}

	cases := []struct {
		name   string
		mutate func(p *layoutalgebra.Problem)
	}{
		{"validate failure", func(p *layoutalgebra.Problem) { p.Tile.DTypeSize = 3 }},
		{"non-power-of-2 shape", func(p *layoutalgebra.Problem) { p.Tile.Shape = []int{6, 8} }},
		{"non-power-of-2 bound", func(p *layoutalgebra.Problem) {
			p.Concurrency[0].Bound = 3
			p.Tile.Shape = []int{4, 6} // keep products consistent-ish; still not pow2
		}},
		{"rank mismatch", func(p *layoutalgebra.Problem) { p.Tile.Shape = []int{4, 4} }},
	}
	for _, c := range cases {
		p := f2AccessProblem()
		c.mutate(&p)
		if _, err := eng.NewSession(p); err == nil {
			t.Errorf("%s: NewSession accepted the problem", c.name)
		}
	}
}

func TestSessionMetadata(t *testing.T) {
	eng, _ := New(F2)
	sess, err := eng.NewSession(f2AccessProblem())
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if sess.Algebra() != F2 {
		t.Errorf("Algebra() = %q, want %q", sess.Algebra(), F2)
	}
	if p := sess.Problem(); p == nil || len(p.Tile.Shape) != 2 {
		t.Errorf("Problem() = %+v, want the 2D fixture", p)
	}
}

// TestSubgroupChainSmoke is the usage-snippet contract end to end on the
// sub-group chain algebra: one Engine, one Session, the three stage
// methods, algebra-agnostic ast.Expr artifacts out.
func TestSubgroupChainSmoke(t *testing.T) {
	eng, err := New(SubgroupChain)
	if err != nil {
		t.Fatalf("New(SubgroupChain): %v", err)
	}
	sess, err := eng.NewSession(layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{7, 16, 4}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 14},
			{Family: "lane", Name: "t", Bound: 32}, // lane last: sgc BoV order
		},
		Lane: "t",
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	for name, stage := range map[string]func() (*Mapping, error){
		"Access":       sess.Access,
		"GmemOptimize": sess.GmemOptimize,
		"SmemOptimize": sess.SmemOptimize,
	} {
		mp, err := stage()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if mp.Algebra != SubgroupChain {
			t.Errorf("%s: Algebra = %q, want %q", name, mp.Algebra, SubgroupChain)
		}
		if mp.Access == nil || mp.Offset == nil {
			t.Errorf("%s: nil artifact in Mapping", name)
		}
		// reference evaluation works with plain variable assignments
		if _, err := ast.Eval(mp.Offset, map[string]int{"w": 3, "t": 17}); err != nil {
			t.Errorf("%s: reference Eval: %v", name, err)
		}
	}
}
