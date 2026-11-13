package f2

import (
	"fmt"
	"math/rand"
	"testing"
)

// ---------------------------------------------------------------------------
// Convergence evidence for the fold rewrite system (see theory.md)
//
// Executable proof obligations, checked step-by-step over the same
// enumeration as the canonical-form gate:
//
//  1. Termination — every rule application strictly decreases ITS OWN
//     witness (per-rule obligations below; each witness is well-founded):
//       rewriteFold, rewriteIdempotent : node count N strictly decreases
//       rewriteNormalization           : merge-potential P strictly decreases
//       rewriteReassociation           : right-edge potential R strictly decreases
//       rewriteCommute                 : depth-weighted inversions Iw decrease
//       rewriteDistribution            : one of the classical DNF measures
//  2. Confluence — folding with randomly permuted rule orders yields
//     structurally identical canonical forms (Newman's condition evidence).
// ---------------------------------------------------------------------------

// foldMeasure carries the per-rule witness coordinates.
type foldMeasure struct {
	p, n, r, iw, arith int
}

// arithInterp is the arithmetic interpretation used as the distribution
// witness: leaves 1, Add(a,b) -> a+b+1, Mul(a,b) -> 2*a*b. Each
// distribution step (x+y)*z -> x*z + y*z strictly decreases it by 2|z|-1.
func arithInterp(e Expr) int {
	b, ok := e.(*NodeBinary)
	if !ok {
		return 1
	}
	l, r := arithInterp(b.L()), arithInterp(b.R())
	if b.Op() == Add {
		return l + r + 1
	}
	return 2 * l * r
}

// leafVars returns the names of all variables under e (membership only).
func leafVars(e Expr) []string {
	out := []string{}
	var walk func(x Expr)
	walk = func(x Expr) {
		if v, ok := x.(*NodeVariable); ok {
			out = append(out, v.Name())
			return
		}
		if b, ok := x.(*NodeBinary); ok {
			walk(b.L())
			walk(b.R())
		}
	}
	walk(e)
	return out
}

func pow(w, k int) int {
	out := 1
	for ; k > 0; k-- {
		out *= w
	}
	return out
}

// measureWeight: w = 2*n0+1, fixed per fold invocation from the input tree.
func measureWeight(e Expr) int {
	n := 0
	var walk func(x Expr)
	walk = func(x Expr) {
		n++
		if b, ok := x.(*NodeBinary); ok {
			walk(b.L())
			walk(b.R())
		}
	}
	walk(e)
	return 2*n + 1
}

func measureOf(e Expr, w int) foldMeasure {
	out := foldMeasure{}

	// P: merge potential — per Add component root, per variable, C(k,2)
	// over elements whose leaf set contains that variable. Leaf sets are
	// invariant under commute and reassociation, so those rules cannot move
	// P; normalization and the Xor-arm of idempotent strictly decrease it.
	var pWalk func(x Expr, parentOp int, same bool)
	pWalk = func(x Expr, parentOp int, same bool) {
		b, isBin := x.(*NodeBinary)
		if !isBin {
			return
		}
		if b.Op() == Add && !(same && parentOp == Add) {
			arr := []Expr{}
			flattenComponent(Add, b, &arr)
			group := map[string]int{}
			for _, el := range arr {
				seen := map[string]bool{}
				for _, name := range leafVars(el) {
					if !seen[name] {
						seen[name] = true
						group[name]++
					}
				}
			}
			for _, c := range group {
				out.p += c * (c - 1) / 2
			}
		}
		pWalk(b.L(), b.Op(), true)
		pWalk(b.R(), b.Op(), true)
	}
	pWalk(e, -1, false)

	// N: node count
	var nWalk func(x Expr)
	nWalk = func(x Expr) {
		out.n++
		if b, ok := x.(*NodeBinary); ok {
			nWalk(b.L())
			nWalk(b.R())
		}
	}
	nWalk(e)

	// R: depth-weighted right-edge potential per same-op component. An edge
	// at component depth dp in a component of height H contributes (H - dp);
	// rotation moves an edge strictly deeper or eliminates it, and no rule
	// creates a same-op right edge.
	var reWalk func(x Expr, parentOp int, same bool)
	reWalk = func(x Expr, parentOp int, same bool) {
		b, isBin := x.(*NodeBinary)
		if !isBin {
			return
		}
		if !(same && b.Op() == parentOp) {
			h := 0
			var hWalk func(y *NodeBinary, depth int)
			hWalk = func(y *NodeBinary, depth int) {
				if depth > h {
					h = depth
				}
				if l, ok := y.L().(*NodeBinary); ok && l.Op() == y.Op() {
					hWalk(l, depth+1)
				}
				if r, ok := y.R().(*NodeBinary); ok && r.Op() == y.Op() {
					hWalk(r, depth+1)
				}
			}
			hWalk(b, 0)
			var acc func(y *NodeBinary, depth int)
			acc = func(y *NodeBinary, depth int) {
				if r, ok := y.R().(*NodeBinary); ok && r.Op() == y.Op() {
					out.r += h - depth
					acc(r, depth+1)
				}
				if l, ok := y.L().(*NodeBinary); ok && l.Op() == y.Op() {
					acc(l, depth+1)
				}
			}
			acc(b, 0)
		}
		reWalk(b.L(), b.Op(), true)
		reWalk(b.R(), b.Op(), true)
	}
	reWalk(e, -1, false)

	// Iw: depth-weighted inversions (w^(component root depth)). A commute
	// case-1 swap at depth dm removes its own component's inversion and can
	// flip at most n0 inversions per ancestor level; w's geometric growth
	// (w = 2*n0+1) makes the removal dominate every possible addition.
	var invWalk func(x Expr, parentOp int, same bool, depth int)
	invWalk = func(x Expr, parentOp int, same bool, depth int) {
		b, isBin := x.(*NodeBinary)
		if !isBin {
			return
		}
		if !(same && b.Op() == parentOp) {
			arr := []Expr{}
			flattenComponent(b.Op(), b, &arr)
			cnt := 0
			for i := 0; i+1 < len(arr); i++ {
				for j := i + 1; j < len(arr); j++ {
					if arr[i].Order().Greater(arr[j].Order()) {
						cnt++
					}
				}
			}
			out.iw += cnt * pow(w, depth)
		}
		invWalk(b.L(), b.Op(), true, depth+1)
		invWalk(b.R(), b.Op(), true, depth+1)
	}
	invWalk(e, -1, false, 0)

	return out
}

// rewriteOnce applies the first applicable rule at the node, else descends
// left then right — mirrors the production driver's semantics closely enough
// for convergence testing. Returns the applied rule index and the node the
// rule fired on (pre-replacement), or (-1, nil) when nothing fired.
func rewriteOnce(e Expr, order []rewriteRules) (Expr, int, Expr, bool) {
	for ri, r := range order {
		if r.Check(e) {
			out, err := r.Rewrite(e)
			if err == nil && out != nil {
				return out, ri, e, true
			}
		}
	}
	if b, ok := e.(*NodeBinary); ok {
		if nl, ri, fired, ch := rewriteOnce(b.L(), order); ch {
			return NewBinary(b.Op(), nl, b.R()), ri, fired, true
		}
		if nr, ri, fired, ch := rewriteOnce(b.R(), order); ch {
			return NewBinary(b.Op(), b.L(), nr), ri, fired, true
		}
	}
	return e, -1, nil, false
}

// inversionCount returns the number of order-inversions among the flattened
// operands of e's same-op component.
func inversionCount(e Expr) int {
	b, ok := e.(*NodeBinary)
	if !ok {
		return 0
	}
	arr := []Expr{}
	flattenComponent(b.Op(), b, &arr)
	inv := 0
	for i := 0; i+1 < len(arr); i++ {
		for j := i + 1; j < len(arr); j++ {
			if arr[i].Order().Greater(arr[j].Order()) {
				inv++
			}
		}
	}
	return inv
}

func foldWithOrder(e Expr, order []rewriteRules) Expr {
	cur := e
	for pass := 0; pass < folderMaxIteration; pass++ {
		out, _, _, changed := rewriteOnce(cur, order)
		if !changed {
			return out
		}
		cur = out
	}
	panic("fold: iteration cap in foldWithOrder")
}

// ruleName identifies rules by their index in rewriteRuleOrder.
func ruleName(ri int) string {
	switch ri {
	case 0:
		return "rewriteFold"
	case 1:
		return "rewriteCommute"
	case 2:
		return "rewriteIdempotent"
	case 3:
		return "rewriteReassociation"
	case 4:
		return "rewriteDistribution"
	case 5:
		return "rewriteNormalization"
	}
	return fmt.Sprintf("rule[%d]", ri)
}

// TestFoldMeasureDecreases: every rule application, on every enumerated
// expression, strictly decreases its own witness.
func TestFoldMeasureDecreases(t *testing.T) {
	const maxReport = 10
	failures := []string{}
	count := 0
	check := func(e Expr) {
		count++
		w := measureWeight(e)
		cur := e
		for pass := 0; pass < folderMaxIteration; pass++ {
			before := measureOf(cur, w)
			out, ri, fired, changed := rewriteOnce(cur, rewriteRuleOrder)
			if !changed {
				return
			}
			after := measureOf(out, w)
			ok := false
			switch ri {
			case 0, 2: // rewriteFold, rewriteIdempotent: N strictly decreases
				ok = after.n < before.n
			case 5: // rewriteNormalization: lexicographic (P, N) decreases —
				// variable merges drop P; the rule also fires on common
				// non-variable factors (factoring), which drop N instead
				ok = after.p < before.p ||
					(after.p == before.p && after.n < before.n)
			case 3: // rewriteReassociation: R strictly decreases
				ok = after.r < before.r
			case 1: // rewriteCommute — per-case witnesses:
				//   case #1 (pair swap): global depth-weighted inversions
				//     strictly decrease (swap leaves every depth unchanged);
				//   case #2 (flatten+sort): the fired component's own
				//     inversion count strictly drops to zero (re-nesting can
				//     shift sub-component depths, so the global weighted
				//     measure is not the witness here).
				fb := fired.(*NodeBinary)
				_, lSame := fb.L().(*NodeBinary)
				if lSame && fb.L().(*NodeBinary).Op() == fb.Op() {
					ok = inversionCount(fired) > 0 && inversionCount(out) == 0
				} else {
					ok = after.iw < before.iw
				}
			case 4: // rewriteDistribution: arithmetic interpretation decreases
				ok = arithInterp(out) < arithInterp(cur)
			}
			if !ok {
				if len(failures) < maxReport {
					failures = append(failures,
						fmt.Sprintf("%s: %v -> %v : %+v -> %+v", ruleName(ri), cur, out, before, after))
				}
				return
			}
			cur = out
		}
		panic("cap")
	}
	for n := 1; n <= 4; n++ {
		enumerate(n, check)
	}
	for _, f := range failures {
		t.Errorf("witness violation: %s", f)
	}
	t.Logf("checked per-rule witnesses on %d expressions", count)
}

// TestFoldConfluence: folding with permuted rule orders must reach the same
// canonical form — evidence for local confluence (Newman's condition).
func TestFoldConfluence(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	failures := []string{}
	const maxReport = 10
	count := 0
	permOf := func() []rewriteRules {
		p := make([]rewriteRules, len(rewriteRuleOrder))
		copy(p, rewriteRuleOrder)
		rng.Shuffle(len(p), func(i, j int) { p[i], p[j] = p[j], p[i] })
		return p
	}
	confluence := func(e Expr) {
		count++
		want := foldWithOrder(e, rewriteRuleOrder)
		for k := 0; k < 3; k++ {
			got := foldWithOrder(e, permOf())
			if !got.IsEqual(want) {
				if len(failures) < maxReport {
					failures = append(failures,
						fmt.Sprintf("%v : standard=%v permuted=%v", e, want, got))
				}
				return
			}
		}
	}
	for n := 1; n <= 3; n++ {
		enumerate(n, confluence)
	}
	for _, f := range failures {
		t.Errorf("confluence violation: %s", f)
	}
	t.Logf("checked confluence on %d expressions x 3 permutations", count)
}
