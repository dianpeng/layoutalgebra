// Command gmem-coalescing is a focused GMEM example: it tunes the
// coalescing window with layout.WithLineBytes and measures, per warp, the
// distinct 32B sectors and cache lines the base access mapping touches
// versus the GMEM-optimized mapping — the before/after numbers a compiler
// backend would feed its cost model.
//
// It exercises: layout.WithLineBytes, NewSession/GmemOptimize on
// layout.SubgroupChain, and ast.Eval over the full lane sweep.
//
// Run: go run ./examples/gmem-coalescing
package main

import (
	"fmt"
	"os"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/pkg/ast"
	"github.com/dianpeng/layoutalgebra/pkg/layout"
)

const (
	lineBytes  = 128 // must match WithLineBytes below
	sectorSize = 32  // architecture 32B sectors
	warp       = 0   // which warp to report
	laneCount  = 32  // lanes swept per warp
)

// sweep evaluates expr at every lane of one warp and returns the distinct
// 32B sectors and distinct lineBytes lines hit.
func sweep(expr ast.Expr, w int) (sectors, lines map[int]bool) {
	sectors, lines = map[int]bool{}, map[int]bool{}
	for t := 0; t < laneCount; t++ {
		off, err := ast.Eval(expr, map[string]int{"w": w, "t": t})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		sectors[off/sectorSize] = true
		lines[off/lineBytes] = true
	}
	return sectors, lines
}

func report(name string, expr ast.Expr) {
	sectors, lines := sweep(expr, warp)
	fmt.Printf("%-8s warp w=%d: %2d lanes touch %2d sectors, %d line(s) of %dB\n",
		name, warp, laneCount, len(sectors), len(lines), lineBytes)
}

func main() {
	// Lane-first BoV on purpose: the lane variable's freedom lands in the
	// major dimension and the warp's 32 lanes scatter across the tile.
	prob := layoutalgebra.Problem{
		Tile: layoutalgebra.Tile{Shape: []int{2, 96}, DTypeSize: 4},
		Concurrency: []layoutalgebra.Variable{
			{Family: "lane", Name: "t", Bound: 32},
			{Family: "warp", Name: "w", Bound: 6},
		},
		Lane: "t",
	}
	eng, err := layout.New(layout.SubgroupChain, layout.WithLineBytes(lineBytes))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sess, err := eng.NewSession(prob)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	base, _ := sess.Access()
	gmem, _ := sess.GmemOptimize()

	fmt.Printf("problem: tile [2,96] dtype 4B, lane-first BoV (t bound 32, w bound 6)\n")
	fmt.Printf("optimized window: last-dim suffix up to min(96, %d/4) elements\n\n", lineBytes)
	report("base:", base.Offset)
	report("gmem:", gmem.Offset)
	fmt.Println("\nthe gmem pass redrew the mapping: lane freedom moved into the")
	fmt.Println("last dimension, so a warp's lanes share one coalesced line")
	fmt.Println("instead of striding across the tile.")
}
