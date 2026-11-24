// Command compilerpass is the headline example for a compiler/backend
// author: it shows the consumer-side "pass" pattern against the universal
// integration layer — each pkg/layout stage artifact (an ast.Expr) becomes
// a pass in YOUR optimization pipeline, evaluated against your own cost
// model via ast.Eval — on both algebras through the same facade.
//
// It exercises: layout.New / NewSession / Access / GmemOptimize /
// SmemOptimize, the one-token algebra switch (layout.F2 <->
// layout.SubgroupChain), ast.Eval reference evaluation, and the
// conservative error contract (a stage that cannot optimize reports its
// verdict instead of silently no-op'ing).
//
// Run: go run ./examples/compilerpass
package main

import (
	"fmt"
	"os"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/pkg/ast"
	"github.com/dianpeng/layoutalgebra/pkg/layout"
)

// offsetPass is the consumer-side pattern this example demonstrates: wrap a
// stage's byte-offset AST together with the problem it was built for, so
// the compiler can evaluate it at concrete hardware coordinates inside its
// own pass pipeline.
type offsetPass struct {
	name      string   // pass name, eg "gmem"
	expr      ast.Expr // the stage's byte-offset AST
	prob      layoutalgebra.Problem
	bitExpand bool // F2 ASTs reference per-bit variables; sgc uses variable names
}

// at evaluates the offset at one (warp, lane) coordinate. F₂ ASTs reference
// per-bit variables (t0, t1, ...), so the variable-level assignment is
// expanded first; sub-group-chain ASTs consume variable names directly.
func (p offsetPass) at(warp, lane int) int {
	assign := map[string]int{"w": warp, "t": lane}
	if p.bitExpand {
		assign = expandBits(p.prob, assign)
	}
	v, err := ast.Eval(p.expr, assign)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compilerpass: eval %s at w=%d t=%d: %v\n", p.name, warp, lane, err)
		os.Exit(1)
	}
	return v
}

// expandBits extends a variable-level assignment with the per-bit entries
// the F₂ algebra's lowered ASTs consume: bit j of variable v is named v+j
// (w0, t1, ...). F2 bounds are powers of two, so the bit count is exact.
func expandBits(p layoutalgebra.Problem, assign map[string]int) map[string]int {
	out := make(map[string]int, len(assign)+8)
	for k, v := range assign {
		out[k] = v
	}
	for _, v := range p.Concurrency {
		for j := 0; 1<<uint(j) < v.Bound; j++ {
			out[fmt.Sprintf("%s%d", v.Name, j)] = (assign[v.Name] >> uint(j)) & 1
		}
	}
	return out
}

// sectorTable prints the (w, t) -> byte offset table for one warp and the
// number of distinct 32B sectors the warp's lanes touch — the GMEM
// transaction cost a compiler's cost model cares about.
func sectorTable(p offsetPass, warp, lanes int) {
	sectors := map[int]bool{}
	fmt.Printf("pass %q, warp w=%d:\n", p.name, warp)
	for t := 0; t < lanes; t++ {
		off := p.at(warp, t)
		sectors[off/32] = true
		fmt.Printf("  t=%2d -> byte offset %3d\n", t, off)
	}
	fmt.Printf("  -> %d distinct 32B sector(s) touched by this warp's lanes\n", len(sectors))
}

func main() {
	// --- F2 algebra, lane-last convention ---
	// The F2 GMEM pass assumes the trailing-lane convention (the LAST
	// laneBits hardware bits are the warp lane bits), so its permutation M
	// is the identity by construction and base/gmem offsets coincide; the
	// stage certifies the coalescing score instead of rewriting.
	f2Prob := layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{4, 8}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 4},
			{Family: "lane", Name: "t", Bound: 8},
		},
		Lane: "t",
	}
	f2Eng, err := layout.New(layout.F2)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	f2Sess, err := f2Eng.NewSession(f2Prob)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	base, _ := f2Sess.Access()
	gmem, _ := f2Sess.GmemOptimize()
	sectorTable(offsetPass{"access", base.Offset, f2Prob, true}, 1, 8)
	sectorTable(offsetPass{"gmem", gmem.Offset, f2Prob, true}, 1, 8)
	// The F2 SMEM stage declines this tiling (no lane bit above the bank
	// window): the conservative contract reports it, execution continues.
	if _, err := f2Sess.SmemOptimize(); err != nil {
		fmt.Printf("f2 smem stage verdict (expected decline): %v\n", err)
	}

	// --- sub-group chain algebra, lane-first (deliberately mis-ordered) ---
	// The one-token switch. Here the BoV order IS the layout: putting the
	// lane first buries lane freedom in the major dimension, and the GMEM
	// pass rewrites the mapping to draw it back into the last dimension —
	// the improvement is visible in the sector counts.
	sgcProb := layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{2, 96}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "lane", Name: "t", Bound: 32},
			{Family: "warp", Name: "w", Bound: 6},
		},
		Lane: "t",
	}
	sgcEng, _ := layout.New(layout.SubgroupChain)
	sgcSess, err := sgcEng.NewSession(sgcProb)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sgcBase, _ := sgcSess.Access()
	sgcGmem, _ := sgcSess.GmemOptimize()
	// sgc ASTs consume variable-level names directly: bitExpand=false.
	sectorTable(offsetPass{"access", sgcBase.Offset, sgcProb, false}, 0, 32)
	sectorTable(offsetPass{"gmem", sgcGmem.Offset, sgcProb, false}, 0, 32)
}
