package ast

import "testing"

func TestEvalConst(t *testing.T) {
	v, err := Eval(NewConst(5), nil)
	if err != nil || v != 5 {
		t.Errorf("Eval(Const 5) = %d, %v; want 5, nil", v, err)
	}
}

func TestEvalVar(t *testing.T) {
	v, err := Eval(NewVar("t", "lane"), map[string]int{"t": 9})
	if err != nil || v != 9 {
		t.Errorf("Eval(Var t) = %d, %v; want 9, nil", v, err)
	}
	if _, err := Eval(NewVar("t", ""), map[string]int{"w": 1}); err == nil {
		t.Error("Eval(Var t) with missing assignment: want error")
	}
}

func TestEvalSymbol(t *testing.T) {
	v, err := Eval(NewSymbol("M"), map[string]int{"M": 12})
	if err != nil || v != 12 {
		t.Errorf("Eval(Symbol M) = %d, %v; want 12, nil", v, err)
	}
	if _, err := Eval(NewSymbol("M"), nil); err == nil {
		t.Error("Eval(Symbol M) with no assignment: want error")
	}
}

func TestEvalBitwise(t *testing.T) {
	// f2 addition/multiplication are XOR/AND on the bit level; on wider ints
	// they are plain bitwise ops
	cases := []struct {
		e    Expr
		want int
	}{
		{NewXor(NewConst(6), NewConst(3)), 5},
		{NewAnd(NewConst(6), NewConst(3)), 2},
		{NewXor(NewConst(1), NewConst(0)), 1},
		{NewAnd(NewConst(1), NewConst(1)), 1},
		{NewAnd(NewConst(1), NewConst(0)), 0},
	}
	for _, c := range cases {
		if v, err := Eval(c.e, nil); err != nil || v != c.want {
			t.Errorf("Eval(%v) = %d, %v; want %d, nil", c.e, v, err, c.want)
		}
	}
}

func TestEvalArithmetic(t *testing.T) {
	cases := []struct {
		e    Expr
		want int
	}{
		{NewAdd(NewConst(7), NewConst(2)), 9},
		{NewSub(NewConst(7), NewConst(2)), 5},
		{NewMul(NewConst(7), NewConst(2)), 14},
		{NewDiv(NewConst(7), NewConst(2)), 3}, // truncating
		{NewDiv(NewConst(8), NewConst(2)), 4},
		{NewMod(NewConst(7), NewConst(2)), 1},
		{NewMod(NewConst(9), NewConst(4)), 1},
	}
	for _, c := range cases {
		if v, err := Eval(c.e, nil); err != nil || v != c.want {
			t.Errorf("Eval(%v) = %d, %v; want %d, nil", c.e, v, err, c.want)
		}
	}
}

func TestEvalDivModByZero(t *testing.T) {
	if _, err := Eval(NewDiv(NewConst(1), NewConst(0)), nil); err == nil {
		t.Error("Eval(Div by 0): want error")
	}
	if _, err := Eval(NewMod(NewConst(1), NewConst(0)), nil); err == nil {
		t.Error("Eval(Mod by 0): want error")
	}
	// the zero divisor must be detected AFTER the operands evaluate: a missing
	// variable in an operand reports the variable, not the division
	_, err := Eval(NewDiv(NewVar("t", ""), NewConst(0)), nil)
	if err == nil {
		t.Fatal("Eval(Div(var, 0)): want error")
	}
}

func TestEvalTupleError(t *testing.T) {
	if _, err := Eval(NewTuple([]Expr{NewConst(1)}), nil); err == nil {
		t.Error("Eval(Tuple): want error (structural node has no scalar value)")
	}
}

func TestEvalNested(t *testing.T) {
	// (t ^ M) + (w * 2)
	e := NewAdd(
		NewXor(NewVar("t", "lane"), NewSymbol("M")),
		NewMul(NewVar("w", "warp"), NewConst(2)),
	)
	assign := map[string]int{"t": 9, "M": 12, "w": 3} // (9^12) + 6 = 5 + 6
	if v, err := Eval(e, assign); err != nil || v != 11 {
		t.Errorf("Eval(nested) = %d, %v; want 11, nil", v, err)
	}
}

func TestEvalModGoSemantics(t *testing.T) {
	// Mod is Go's %: pin the sign behavior so it matches the subgroupchain
	// reference interpreter exactly (both delegate to Go's % operator)
	if v, err := Eval(NewMod(NewConst(-7), NewConst(3)), nil); err != nil || v != -1 {
		t.Errorf("Eval(-7 %% 3) = %d, %v; want -1, nil", v, err)
	}
}
