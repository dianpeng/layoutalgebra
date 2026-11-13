package f2

import (
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// Fold completeness enumeration gate
//
// Generates every expression up to a fixed leaf count over a small alphabet
// and asserts the fold engine reaches the canonical form:
//
//  1. no same-op right-nesting survives (rewriteReassociation's postcondition)
//  2. every same-op chain is order-sorted (rewriteCommute's postcondition)
//  3. fold is idempotent (a second pass changes nothing)
//
// Adding or changing a rewrite rule must keep this gate green — it is the
// falsifiable completeness contract for the fold engine.
// ---------------------------------------------------------------------------

func canonLeaves() []Expr {
	return []Expr{
		NewVariable("x"),
		NewVariable("y"),
		NewSymbol("S"),
		NewConstZero(),
	}
}

// enumerate calls fn with every expression having exactly n leaves over the
// canonLeaves alphabet, covering all binary-tree shapes and operator
// assignments.
func enumerate(n int, fn func(Expr)) {
	leaves := canonLeaves()
	var gen func(n int, emit func(Expr))
	gen = func(n int, emit func(Expr)) {
		if n == 1 {
			for _, l := range leaves {
				emit(l)
			}
			return
		}
		for i := 1; i < n; i++ {
			i := i
			gen(i, func(l Expr) {
				gen(n-i, func(r Expr) {
					emit(NewBinary(Xor, l, r))
					emit(NewBinary(Mul, l, r))
				})
			})
		}
	}
	gen(n, fn)
}

// canonViolation returns a description of the first canonical-form violation
// in e, or "" if e is canonical per the fold engine's intended normal form:
// no right-nested same-op chains, and every same-op component flattened and
// order-sorted.
func canonViolation(e Expr) string {
	bad := ""
	VisitPost(
		e,
		func(n Expr) bool { return n.Kind() == Binary },
		func(n Expr) error {
			if bad != "" {
				return nil
			}
			bin := n.(*NodeBinary)
			if r, ok := bin.R().(*NodeBinary); ok && r.Op() == bin.Op() {
				bad = fmt.Sprintf("right-nested same-op chain at %v", bin)
				return nil
			}
			op := bin.Op()
			arr := []Expr{}
			var flatten func(x Expr)
			flatten = func(x Expr) {
				if b, ok := x.(*NodeBinary); ok && b.Op() == op {
					flatten(b.L())
					flatten(b.R())
					return
				}
				arr = append(arr, x)
			}
			flatten(bin)
			for i := 0; i+1 < len(arr); i++ {
				if arr[i].Order().Greater(arr[i+1].Order()) {
					bad = fmt.Sprintf("unsorted same-op chain at %v", bin)
					return nil
				}
			}
			return nil
		},
	)
	return bad
}

func TestFoldCanonicalEnumeration(t *testing.T) {
	const maxLeaves = 4
	const maxReport = 10
	failures := []string{}
	count := 0
	for n := 1; n <= maxLeaves; n++ {
		enumerate(n, func(e Expr) {
			count++
			f1 := Fold(e)
			if v := canonViolation(f1); v != "" {
				if len(failures) < maxReport {
					failures = append(failures, fmt.Sprintf("%v -> %v (%s)", e, f1, v))
				}
			}
			f2 := Fold(f1)
			if !f1.IsEqual(f2) {
				if len(failures) < maxReport {
					failures = append(failures, fmt.Sprintf("not idempotent: %v -> %v -> %v", e, f1, f2))
				}
			}
		})
	}
	for _, f := range failures {
		t.Errorf("canonical-form violation: %s", f)
	}
	if len(failures) > maxReport {
		t.Errorf("... and %d more violations (only %d shown)", len(failures)-maxReport, maxReport)
	}
	t.Logf("enumerated %d expressions up to %d leaves", count, maxLeaves)
}
