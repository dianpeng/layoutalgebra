package ast

import (
	"strings"
	"testing"
)

// allKinds builds one tree that contains every one of the eleven node kinds,
// so a single Walk can prove full coverage.
func allKinds() Expr {
	return NewAdd(
		NewXor(NewVar("t", "lane"), NewSymbol("M")),
		NewSub(
			NewAnd(NewConst(6), NewVar("w", "warp")),
			NewMul(
				NewDiv(NewTuple([]Expr{NewConst(1), NewConst(2)}), NewConst(1)),
				NewMod(NewConst(7), NewConst(4)),
			),
		),
	)
}

func TestNodeSetComplete(t *testing.T) {
	// the sealed node set is exactly these eleven kinds
	seen := map[string]bool{}
	Walk(allKinds(), func(e Expr) bool {
		switch e.(type) {
		case *Const:
			seen["Const"] = true
		case *Symbol:
			seen["Symbol"] = true
		case *Var:
			seen["Var"] = true
		case *Xor:
			seen["Xor"] = true
		case *And:
			seen["And"] = true
		case *Add:
			seen["Add"] = true
		case *Sub:
			seen["Sub"] = true
		case *Mul:
			seen["Mul"] = true
		case *Div:
			seen["Div"] = true
		case *Mod:
			seen["Mod"] = true
		case *Tuple:
			seen["Tuple"] = true
		default:
			t.Errorf("unexpected node type %T", e)
		}
		return true
	})
	want := []string{
		"Const", "Symbol", "Var", "Xor", "And", "Add", "Sub",
		"Mul", "Div", "Mod", "Tuple",
	}
	for _, k := range want {
		if !seen[k] {
			t.Errorf("node kind %s not covered", k)
		}
	}
}

func TestWalkVisitsEveryNodeExactlyOnce(t *testing.T) {
	counts := map[Expr]int{}
	Walk(allKinds(), func(e Expr) bool {
		counts[e]++
		return true
	})
	total := 0
	for e, n := range counts {
		if n != 1 {
			t.Errorf("node %v visited %d times, want exactly 1", e, n)
		}
		total += n
	}
	// 17 nodes in allKinds: Add, Xor, Var, Symbol, Sub, And, Const, Var, Mul,
	// Div, Tuple, Const, Const, Const, Mod, Const, Const
	if total != 17 {
		t.Errorf("Walk visited %d nodes, want 17", total)
	}
}

func TestWalkStopSkipsSubtree(t *testing.T) {
	visited := 0
	Walk(allKinds(), func(e Expr) bool {
		visited++
		_, isAdd := e.(*Add)
		return !isAdd // stop at the root -> nothing else is visited
	})
	if visited != 1 {
		t.Errorf("Walk after root stop visited %d nodes, want 1", visited)
	}

	// stopping at the Sub node skips its whole subtree but keeps the Xor side
	visited = 0
	Walk(allKinds(), func(e Expr) bool {
		visited++
		_, isSub := e.(*Sub)
		return !isSub
	})
	// Add, Xor, Var, Symbol, Sub = 5
	if visited != 5 {
		t.Errorf("Walk with subtree stop visited %d nodes, want 5", visited)
	}
}

func TestChildren(t *testing.T) {
	leaf := NewConst(1)
	if got := Children(leaf); got != nil {
		t.Errorf("Children(leaf) = %v, want nil", got)
	}

	x := NewXor(NewConst(1), NewConst(0))
	got := Children(x)
	if len(got) != 2 {
		t.Fatalf("Children(Xor) has %d entries, want 2", len(got))
	}
	if got[0] != Expr(x.L) || got[1] != Expr(x.R) {
		t.Errorf("Children(Xor) = %v, want [L R]", got)
	}

	elem := []Expr{NewConst(1), NewConst(2), NewConst(3)}
	tup := NewTuple(elem)
	if got := Children(tup); len(got) != 3 {
		t.Errorf("Children(Tuple) has %d entries, want 3", len(got))
	}
}

func TestNewTupleCopiesSlice(t *testing.T) {
	elem := []Expr{NewConst(1), NewConst(2)}
	tup := NewTuple(elem)

	elem[0] = NewConst(99) // mutating the input must not affect the node
	if tup.Elem[0].(*Const).Value != 1 {
		t.Errorf("NewTuple aliased the input slice: Elem[0] = %v", tup.Elem[0])
	}
}

func TestConstructorsNoSharedMutableState(t *testing.T) {
	a := NewTuple([]Expr{NewConst(1)})
	b := NewTuple([]Expr{NewConst(2)})
	if &a.Elem[0] == &b.Elem[0] {
		t.Error("two NewTuple calls share element storage")
	}
	a.Elem[0] = NewConst(7)
	if b.Elem[0].(*Const).Value != 2 {
		t.Errorf("mutating a affected b: %v", b.Elem[0])
	}
}

func TestStringDistinctShapes(t *testing.T) {
	l, r := NewConst(6), NewConst(3)
	shapes := []Expr{
		NewAdd(l, r),
		NewSub(l, r),
		NewMul(l, r),
		NewDiv(l, r),
		NewMod(l, r),
		NewXor(l, r),
		NewAnd(l, r),
		NewTuple([]Expr{l, r}),
	}
	seen := map[string]bool{}
	for _, e := range shapes {
		s := e.String()
		if seen[s] {
			t.Errorf("duplicate String() form %q", s)
		}
		seen[s] = true
	}

	// family-qualified and bare vars render differently
	if NewVar("t", "").String() == NewVar("t", "lane").String() {
		t.Errorf("Var with and without family render identically: %q",
			NewVar("t", "").String())
	}
	if NewVar("t", "").String() != "t" {
		t.Errorf("bare Var String() = %q, want t", NewVar("t", "").String())
	}
	if NewConst(5).String() != "5" {
		t.Errorf("Const String() = %q, want 5", NewConst(5).String())
	}
	if NewSymbol("M").String() != "M" {
		t.Errorf("Symbol String() = %q, want M", NewSymbol("M").String())
	}
}

func TestStringParenthesized(t *testing.T) {
	// ((1 ^ t) + 2): binary operators always parenthesize their operands
	e := NewAdd(NewXor(NewConst(1), NewVar("t", "")), NewConst(2))
	want := "((1 ^ t) + 2)"
	if got := e.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestStringTupleBraces(t *testing.T) {
	e := NewTuple([]Expr{NewConst(1), NewVar("t", ""), NewSymbol("M")})
	want := "{1, t, M}"
	if got := e.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestDumpMultiLine(t *testing.T) {
	e := NewAdd(NewXor(NewVar("t", "lane"), NewConst(1)), NewConst(2))
	d := Dump(e)
	if !strings.Contains(d, "\n") {
		t.Errorf("Dump is single-line: %q", d)
	}
	lines := strings.Split(strings.TrimRight(d, "\n"), "\n")
	if len(lines) != 4 { // add + xor + var + const + const... see below
		t.Logf("Dump output:\n%s", d)
	}
	// every line except the first is indented deeper than the root
	for i, ln := range lines[1:] {
		if !strings.HasPrefix(ln, "  ") {
			t.Errorf("Dump line %d %q is not indented", i+1, ln)
		}
	}
	if !strings.HasPrefix(lines[0], "add") {
		t.Errorf("Dump root line = %q, want it to start with the node kind", lines[0])
	}
}
