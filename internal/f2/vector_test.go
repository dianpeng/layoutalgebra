package f2

import (
	"sort"
	"testing"
)

// TestVectorGather proves Gather selects source positions and orders the
// result by the index list: Gather([a,b,c], [2,0,1]) == [c,a,b].
func TestVectorGather(t *testing.T) {
	vec := NewVector([]Expr{
		NewVariable("a"),
		NewVariable("b"),
		NewVariable("c"),
	})

	got := vec.Gather([]int{2, 0, 1})
	if got.Size() != 3 {
		t.Fatalf("Gather size = %d, want 3", got.Size())
	}
	if !got.Extent(0).IsEqual(NewVariable("c")) {
		t.Errorf("Gather[0] structurally non-equal to c")
	}
	if !got.Extent(1).IsEqual(NewVariable("a")) {
		t.Errorf("Gather[1] structurally non-equal to a")
	}
	if !got.Extent(2).IsEqual(NewVariable("b")) {
		t.Errorf("Gather[2] structurally non-equal to b")
	}
}

// TestVectorGatherEmpty proves an empty index list yields a 0-size vector.
func TestVectorGatherEmpty(t *testing.T) {
	vec := NewVector([]Expr{NewVariable("a"), NewVariable("b")})
	got := vec.Gather([]int{})
	if got.Size() != 0 {
		t.Errorf("Gather([]) size = %d, want 0", got.Size())
	}
}

// TestSmokeFoldInspect is ROADMAP success criterion 4: build a composite
// expression ((M*x) ^ (N*y)) ^ (x*x), fold it, and inspect the result —
// the x*x term must fold to x (idempotent), the full result must be
// structurally Xor(Xor(Mul(M,x), Mul(N,y)), x), and a visitor walk must
// collect exactly {x, y} with symbols M, N untouched. Must not panic.
func TestSmokeFoldInspect(t *testing.T) {
	x := NewVariable("x")
	y := NewVariable("y")
	m := NewSymbol("M")
	n := NewSymbol("N")

	expr := NewAdd(
		NewAdd(
			NewMul(m, x),
			NewMul(n, y),
		),
		NewMul(x, x),
	)

	folded := Fold(expr)

	want := NewAdd(
		NewAdd(
			NewMul(NewSymbol("M"), NewVariable("x")),
			NewMul(NewSymbol("N"), NewVariable("y")),
		),
		NewVariable("x"),
	)
	if !folded.IsEqual(want) {
		t.Errorf("smoke: folded result structurally non-equal to Xor(Xor(M*x, N*y), x)")
	}

	// Walk the result, logging every node and collecting variable names.
	varNames := map[string]bool{}
	var symbolNames []string
	err := Visit(
		folded,
		func(e Expr) bool { return true },
		func(e Expr) error {
			switch node := e.(type) {
			case *NodeVariable:
				varNames[node.Name()] = true
			case *NodeSymbol:
				symbolNames = append(symbolNames, node.Name())
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("smoke: visit failed: %v", err)
	}

	sort.Strings(symbolNames)
	t.Logf("smoke: variables=%v symbols=%v", sortedKeys(varNames), symbolNames)

	if len(varNames) != 2 || !varNames["x"] || !varNames["y"] {
		t.Errorf("smoke: variable set = %v, want exactly {x, y}", sortedKeys(varNames))
	}
	if len(symbolNames) != 2 || symbolNames[0] != "M" || symbolNames[1] != "N" {
		t.Errorf("smoke: symbols = %v, want [M N] untouched", symbolNames)
	}
}

// sortedKeys returns the sorted keys of a string set — tiny local helper.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
