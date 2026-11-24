// Command smem-swizzle is a focused SMEM bank-conflict example: it sets the
// bank analysis window with layout.WithBankWindow and prints a
// bank-conflict report — the distinct SMEM banks one warp's lanes hit,
// computed by evaluating the F2 stage AST with ast.Eval over the full lane
// sweep — for two tilings: one already conflict-free and one the swizzle
// stage must conservatively decline.
//
// It exercises: layout.WithBankWindow, NewSession/Access/SmemOptimize on
// layout.F2, ast.Eval with per-bit (F2) assignment expansion, and the
// conservative error contract (a declined stage returns a verdict error,
// never a silently unoptimized result).
//
// Run: go run ./examples/smem-swizzle
package main

import (
	"fmt"
	"os"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/pkg/ast"
	"github.com/dianpeng/layoutalgebra/pkg/layout"
)

// expandBits extends a variable-level assignment with the per-bit entries
// F2 ASTs consume: bit j of variable v is named v+j (t0, t1, ...).
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

// bankReport evaluates the access stage's byte offset for every lane of
// warp 0 and returns the distinct banks hit: byte bits [skip, skip+range).
func bankReport(sess *layout.Session, prob layoutalgebra.Problem, skip, rng int) int {
	access, err := sess.Access()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	lane := prob.Variable(prob.Lane)
	banks := map[int]bool{}
	for t := 0; t < lane.Bound && t < 1<<(rng+1); t++ {
		assign := expandBits(prob, map[string]int{"w": 0, lane.Name: t})
		off, err := ast.Eval(access.Offset, assign)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		banks[(off>>skip)&((1<<rng)-1)] = true
	}
	return len(banks)
}

func run(name string, prob layoutalgebra.Problem, skip, rng int) {
	fmt.Printf("--- %s: tile %v dtype %dB ---\n", name, prob.Tile.Shape, prob.Tile.DTypeSize)
	eng, err := layout.New(layout.F2, layout.WithBankWindow(skip, rng))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sess, err := eng.NewSession(prob)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("banks hit by one warp's lanes (window bits [%d,%d)): %d of %d\n",
		skip, skip+rng, bankReport(sess, prob, skip, rng), 1<<rng)
	_, err = sess.SmemOptimize()
	switch {
	case err == nil:
		fmt.Println("smem stage: bank window already fully ranked — conflict-free,")
		fmt.Println("            mapping returned unchanged (saturated success)")
	default:
		fmt.Printf("smem stage verdict: %v\n", err)
		fmt.Println("conservative contract: no free lane bit above the bank window,")
		fmt.Println("so the XOR swizzle declines instead of guessing")
	}
	fmt.Println()
}

func main() {
	const skip, rng = 2, 5 // CUDA: 32 banks x 4B words, byte bits [2,7)

	// Conflict-free: 64 lanes over a [4,32] tile — every lane lands in its
	// own bank, the swizzle has nothing to add.
	run("saturated tiling", layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{4, 32}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 2},
			{Family: "lane", Name: "t", Bound: 64},
		},
		Lane: "t",
	}, skip, rng)

	// Conflicted: 8 lanes over a [4,8] tile — four lanes share each bank
	// (8 of 32 banks hit), and no lane bit exists above the bank window,
	// so the stage declines loudly.
	run("conflicted tiling", layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{4, 8}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "warp", Name: "w", Bound: 4},
			{Family: "lane", Name: "t", Bound: 8},
		},
		Lane: "t",
	}, skip, rng)

	fmt.Println("note: layout.WithBankBytes tunes the sub-group chain's")
	fmt.Println("SmemCoalesce.BankBytes; the F2 backend takes its window")
	fmt.Println("from WithBankWindow (skip, range) alone.")
}
