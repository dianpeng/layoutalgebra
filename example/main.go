// Command example is a runnable tour of the layoutalgebra universal
// integration layer (pkg/layout + pkg/ast): it drives both algebras through
// the same facade — layout.New, NewSession, then Access / GmemOptimize /
// SmemOptimize on the Session — and prints the algebra-agnostic artifacts
// via Offset.String(), ast.Dump, and ast.Eval on fixed assignments.
//
// Run it with: go run ./example
package main

import (
	"fmt"
	"os"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/pkg/ast"
	"github.com/dianpeng/layoutalgebra/pkg/layout"
)

func main() {
	f2Demo()
	sgcDemo()
}

// fatalf reports an UNexpected failure. Expected stage verdicts — the
// conservative contract declining a tiling — are printed informationally
// by the callers instead; only a broken fixture reaches this.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "example: "+format+"\n", args...)
	os.Exit(1)
}

// expandBits extends a variable-level assignment with the per-bit entries
// the F₂ algebra's lowered ASTs consume: bit j of variable v is named
// v.Name+j (w0, t1, ...). F₂ bounds are powers of two, so the bit count is
// exact. Sub-group-chain ASTs take variable-level names directly and need
// no expansion.
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

// f2Demo drives the F₂ algebra (bit-level, power-of-2 tiles) on two
// problems: one whose SMEM stage conservatively declines, and one sized so
// it saturates cleanly.
func f2Demo() {
	fmt.Println("=== F2 algebra (layout.F2) ===")

	// Defaults only. layout.New also takes tuning options — WithLineBytes,
	// WithBankBytes, WithBankWindow — over the CUDA defaults used here.
	eng, err := layout.New(layout.F2)
	if err != nil {
		fatalf("layout.New(F2): %v", err)
	}

	// f2A: 4x8 tile of 4B elements, 4 warps x 8 lanes.
	f2A := layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{4, 8}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 4},
			{Family: "lane", Name: "t", Bound: 8},
		},
		Lane: "t",
	}
	fmt.Println("problem: tile [4,8] dtype 4B, warp w bound 4, lane t bound 8")
	sess, err := eng.NewSession(f2A)
	if err != nil {
		fatalf("NewSession(f2A): %v", err)
	}
	access, err := sess.Access()
	if err != nil {
		fatalf("Access: %v", err)
	}
	fmt.Printf("Access offset: %s\n", access.Offset.String())
	gmem, err := sess.GmemOptimize()
	if err != nil {
		fatalf("GmemOptimize: %v", err)
	}
	fmt.Printf("GmemOptimize offset: %s\n", gmem.Offset.String())

	// F₂ ASTs are per-bit, so the variable-level assignment is expanded
	// before reference evaluation.
	off, err := ast.Eval(gmem.Offset, expandBits(f2A, map[string]int{"w": 2, "t": 5}))
	if err != nil {
		fatalf("Eval(f2A gmem): %v", err)
	}
	fmt.Printf("byte offset at w=2 t=5: %d\n", off)

	// The conservative contract in action: this tiling leaves no free lane
	// bit above the bank window, so the SMEM stage declines — printed as
	// information, execution continues.
	if _, err := sess.SmemOptimize(); err != nil {
		fmt.Printf("SmemOptimize: not applicable to this tiling (conservative contract): %v\n", err)
	}

	// f2B: 4x32 tile, 2 warps x 64 lanes — the bank window sits inside the
	// lane-bit run, so the same stage saturates cleanly.
	f2B := layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{4, 32}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 2},
			{Family: "lane", Name: "t", Bound: 64},
		},
		Lane: "t",
	}
	fmt.Println("problem: tile [4,32] dtype 4B, warp w bound 2, lane t bound 64")
	sessB, err := eng.NewSession(f2B)
	if err != nil {
		fatalf("NewSession(f2B): %v", err)
	}
	smem, err := sessB.SmemOptimize()
	if err != nil {
		fatalf("SmemOptimize(f2B): %v", err)
	}
	fmt.Printf("SmemOptimize offset: %s\n", smem.Offset.String())
}

// sgcDemo drives the sub-group chain algebra (arbitrary tile
// factorizations) on two problems: the lane-last ordering the fast paths
// like, and a lane-first ordering that forces the GMEM optimizer to
// actually rewrite the mapping.
func sgcDemo() {
	fmt.Println("=== sub-group chain algebra (layout.SubgroupChain) ===")

	eng, err := layout.New(layout.SubgroupChain)
	if err != nil {
		fatalf("layout.New(SubgroupChain): %v", err)
	}

	// sgcA: 7x16x4 tile of 4B elements, 14 warps x 32 lanes, lane variable
	// LAST in the bag (the BoV order the algebra expects).
	sgcA := layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{7, 16, 4}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 14},
			{Family: "lane", Name: "t", Bound: 32}, // lane last: sgc BoV order
		},
		Lane: "t",
	}
	fmt.Println("problem: tile [7,16,4] dtype 4B, warp w bound 14, lane t bound 32")
	sess, err := eng.NewSession(sgcA)
	if err != nil {
		fatalf("NewSession(sgcA): %v", err)
	}
	access, err := sess.Access()
	if err != nil {
		fatalf("Access: %v", err)
	}
	fmt.Printf("Access offset: %s\n", access.Offset.String())
	gmem, err := sess.GmemOptimize()
	if err != nil {
		fatalf("GmemOptimize: %v", err)
	}
	fmt.Printf("GmemOptimize offset: %s\n", gmem.Offset.String())
	smem, err := sess.SmemOptimize()
	if err != nil {
		fatalf("SmemOptimize: %v", err)
	}
	fmt.Printf("SmemOptimize offset: %s\n", smem.Offset.String())

	// Sub-group-chain ASTs consume variable-level names directly — no bit
	// expansion.
	off, err := ast.Eval(gmem.Offset, map[string]int{"w": 3, "t": 17})
	if err != nil {
		fatalf("Eval(sgcA gmem): %v", err)
	}
	fmt.Printf("byte offset at w=3 t=17: %d\n", off)

	fmt.Println("GmemOptimize offset tree (ast.Dump):")
	fmt.Print(ast.Dump(gmem.Offset))

	// sgcB: lane variable FIRST — the lane freedom lands in the major
	// dimension and GMEM coalescing is lost.
	sgcB := layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{2, 96}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "lane", Name: "t", Bound: 32},
			{Family: "warp", Name: "w", Bound: 6},
		},
		Lane: "t",
	}
	fmt.Println("problem: tile [2,96] dtype 4B, lane t bound 32, warp w bound 6")
	sessB, err := eng.NewSession(sgcB)
	if err != nil {
		fatalf("NewSession(sgcB): %v", err)
	}
	gmemB, err := sessB.GmemOptimize()
	if err != nil {
		fatalf("GmemOptimize(sgcB): %v", err)
	}
	fmt.Printf("GmemOptimize offset: %s\n", gmemB.Offset.String())
	fmt.Println("note: the optimizer rewrote the mapping (drew lane freedom into the last dimension) because the lane-first ordering lost coalescing")
}
