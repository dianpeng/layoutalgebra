package subgroupchain

import "testing"

func TestFactoryNodeBasics(t *testing.T) {
	f := &Factory{}

	v := f.Variable("warp", "warp0", 14)
	if v.Kind() != KindVariable {
		t.Errorf("variable kind = %d, want KindVariable", v.Kind())
	}
	if v.UpperBound() != 14 || v.Name() != "warp0" || v.Family() != "warp" {
		t.Errorf("variable fields wrong: ub=%d name=%s family=%s", v.UpperBound(), v.Name(), v.Family())
	}

	c := f.Const(42)
	if c.Kind() != KindConst || c.Value() != 42 {
		t.Errorf("const wrong: kind=%d value=%d", c.Kind(), c.Value())
	}

	b := f.Binary(Mul, v, c)
	if b.Kind() != KindBinary || b.Op() != Mul {
		t.Errorf("binary wrong: kind=%d op=%d", b.Kind(), b.Op())
	}

	v2 := f.Variable("warp", "warp1", 3)
	s := f.Stride([]StrideOffset{
		{CoordinateExpression: v, UpperBound: 14},
		{CoordinateExpression: v2, UpperBound: 3},
	})
	strideNode, ok := s.(*Stride)
	if !ok {
		t.Fatalf("2-element Stride returned %T, want *Stride", s)
	}
	if s.Kind() != KindStride {
		t.Errorf("stride kind = %d, want KindStride", s.Kind())
	}
	if strideNode.Rank() != 42 {
		t.Errorf("stride rank = %d, want 42", strideNode.Rank())
	}
	if len(strideNode.Shape()) != 2 || strideNode.Shape()[0] != 14 || strideNode.Shape()[1] != 3 {
		t.Errorf("stride shape = %v, want [14 3]", strideNode.Shape())
	}
	// row-major strides for shape [14, 3] are [3, 1]
	if len(strideNode.Stride()) != 2 || strideNode.Stride()[0] != 3 || strideNode.Stride()[1] != 1 {
		t.Errorf("stride vector = %v, want [3 1]", strideNode.Stride())
	}

	// single-element Stride is the identity (returns the coordinate itself)
	identity := f.Stride([]StrideOffset{{CoordinateExpression: v, UpperBound: 14}})
	if !identity.IsEqual(v) {
		t.Errorf("1-element Stride = %v, want the coordinate itself", identity)
	}

	// ids are unique and monotonically increasing
	if !(v.Id() < c.Id() && c.Id() < b.Id() && b.Id() < s.Id()) {
		t.Errorf("ids not unique/increasing: v=%d c=%d b=%d s=%d", v.Id(), c.Id(), b.Id(), s.Id())
	}

	// marker is independent from id
	v.SetMarker(99)
	if v.Marker() != 99 || v.Id() == uint64(99) {
		t.Errorf("marker/id alias: marker=%d id=%d", v.Marker(), v.Id())
	}
}

func TestIsEqual(t *testing.T) {
	f := &Factory{}
	v1 := f.Variable("warp", "warp0", 14)
	v2 := f.Variable("warp", "warp0", 14)
	v3 := f.Variable("lane", "lane0", 32)

	if !v1.IsEqual(v2) {
		t.Error("same-name variables reported unequal")
	}
	if v1.IsEqual(v3) {
		t.Error("different-name variables reported equal")
	}
	if v1.IsEqual(f.Const(14)) {
		t.Error("variable vs const reported equal")
	}

	c1 := f.Const(7)
	c2 := f.Const(7)
	c3 := f.Const(8)
	if !c1.IsEqual(c2) || c1.IsEqual(c3) {
		t.Error("const equality broken")
	}

	b1 := f.Binary(Add, v1, c1)
	b2 := f.Binary(Add, v2, c2)
	b3 := f.Binary(Sub, v1, c1)
	if !b1.IsEqual(b2) {
		t.Error("structurally equal binaries reported unequal")
	}
	if b1.IsEqual(b3) {
		t.Error("different-op binaries reported equal")
	}

	// notes: 1-element Stride is the identity (returns the coordinate), so
	// stride-equality tests use 2-element strides
	v4 := f.Variable("warp", "warp9", 3)
	s1 := f.Stride([]StrideOffset{
		{CoordinateExpression: v1, UpperBound: 14},
		{CoordinateExpression: v4, UpperBound: 3},
	})
	s2 := f.Stride([]StrideOffset{
		{CoordinateExpression: v2, UpperBound: 14},
		{CoordinateExpression: v4, UpperBound: 3},
	})
	s3 := f.Stride([]StrideOffset{
		{CoordinateExpression: v1, UpperBound: 7},
		{CoordinateExpression: v4, UpperBound: 6},
	})
	if !s1.IsEqual(s2) {
		t.Error("equal strides reported unequal")
	}
	if s1.IsEqual(s3) {
		t.Error("different-shape strides reported equal")
	}
}

func TestFactoryProduct(t *testing.T) {
	f := &Factory{}
	c2, c3, c4 := f.Const(2), f.Const(3), f.Const(4)

	p := f.Product(c2, c3, c4)
	if got := evalExpr(t, p, nil); got != 24 {
		t.Errorf("Product(2,3,4) = %d, want 24", got)
	}

	single := f.Product(c2)
	if got := evalExpr(t, single, nil); got != 2 {
		t.Errorf("Product(2) = %d, want 2", got)
	}
}

func TestFactorizeBasic(t *testing.T) {
	f := &Factory{}
	v := f.Variable("t", "t", 32)
	AnalyzeRange(v)

	// 32 = 8 * 4 mixed radix: digits are t/4 (range [0,8)) and t%4 (range [0,4))
	digits, err := f.Factorize(v, []int{8, 4})
	if err != nil {
		t.Fatalf("Factorize failed: %v", err)
	}
	if len(digits) != 2 {
		t.Fatalf("got %d digits, want 2", len(digits))
	}
	for x := 0; x < 32; x++ {
		env := map[string]int{"t": x}
		d0, d1 := evalExpr(t, digits[0], env), evalExpr(t, digits[1], env)
		if d0 != x/4 || d1 != x%4 {
			t.Fatalf("x=%d: digits = (%d, %d), want (%d, %d)", x, d0, d1, x/4, x%4)
		}
		if d0 < 0 || d0 >= 8 || d1 < 0 || d1 >= 4 {
			t.Fatalf("x=%d: digit out of range: (%d, %d)", x, d0, d1)
		}
	}

	// ranges attached to the digits
	if r, ok := digits[0].Range().(*RangeInterval); !ok || r.UpperBound() != 8 {
		t.Errorf("digit0 range = %v, want interval [0,8)", digits[0].Range())
	}
	if r, ok := digits[1].Range().(*RangeInterval); !ok || r.UpperBound() != 4 {
		t.Errorf("digit1 range = %v, want interval [0,4)", digits[1].Range())
	}
}

func TestFactorizeThreeRadixWithTrivialMiddle(t *testing.T) {
	f := &Factory{}
	v := f.Variable("t", "t", 8)
	AnalyzeRange(v)

	// 8 = 2 * 1 * 4: the middle digit is trivial and must yield a nil entry
	digits, err := f.Factorize(v, []int{2, 1, 4})
	if err != nil {
		t.Fatalf("Factorize failed: %v", err)
	}
	if len(digits) != 3 {
		t.Fatalf("got %d digits, want 3", len(digits))
	}
	if digits[1] != nil {
		t.Errorf("trivial middle digit = %v, want nil", digits[1])
	}
	for x := 0; x < 8; x++ {
		env := map[string]int{"t": x}
		if d0, d2 := evalExpr(t, digits[0], env), evalExpr(t, digits[2], env); d0 != x/4 || d2 != x%4 {
			t.Fatalf("x=%d: digits = (%d, %d), want (%d, %d)", x, d0, d2, x/4, x%4)
		}
	}
}

func TestFactorizeLeadingTrivial(t *testing.T) {
	f := &Factory{}
	v := f.Variable("t", "t", 6)
	AnalyzeRange(v)

	// 6 = 1 * 6: the high digit is trivial
	digits, err := f.Factorize(v, []int{1, 6})
	if err != nil {
		t.Fatalf("Factorize failed: %v", err)
	}
	if digits[0] != nil {
		t.Errorf("trivial leading digit = %v, want nil", digits[0])
	}
	for x := 0; x < 6; x++ {
		if got := evalExpr(t, digits[1], map[string]int{"t": x}); got != x {
			t.Fatalf("x=%d: digit = %d, want %d", x, got, x)
		}
	}
}

func TestFactorizeErrors(t *testing.T) {
	f := &Factory{}

	// no range attached
	bare := f.Variable("t", "t", 8)
	if _, err := f.Factorize(bare, []int{8}); err == nil {
		t.Error("Factorize without range: expected error")
	}

	// unknown (non-interval) range
	bare.SetRange(NewUnknown())
	if _, err := f.Factorize(bare, []int{8}); err == nil {
		t.Error("Factorize with unknown range: expected error")
	}

	// pointless: upper bound 1 (range overridden to bypass construction guard)
	one := f.Variable("t", "t1", 8)
	one.SetRange(NewInterval(1))
	if _, err := f.Factorize(one, []int{1, 1}); err == nil {
		t.Error("Factorize with upper bound 1: expected error")
	}

	// mismatched product
	v := f.Variable("t", "t2", 8)
	AnalyzeRange(v)
	if _, err := f.Factorize(v, []int{4, 4}); err == nil {
		t.Error("Factorize with mismatched product: expected error")
	}

	// empty bounds
	if _, err := f.Factorize(v, nil); err == nil {
		t.Error("Factorize with empty bounds: expected error")
	}

	// invalid bound entries
	if _, err := f.Factorize(v, []int{0, 8}); err == nil {
		t.Error("Factorize with bound 0: expected error")
	}
	if _, err := f.Factorize(v, []int{-2, 4}); err == nil {
		t.Error("Factorize with negative bound: expected error")
	}
}

func TestVariableRejectsTrivialBound(t *testing.T) {
	f := &Factory{}
	defer func() {
		if recover() == nil {
			t.Error("Variable with upper bound 0: expected panic")
		}
	}()
	f.Variable("t", "t", 0)
}

func TestStrideEmptyShape(t *testing.T) {
	f := &Factory{}
	s := f.Stride(nil)
	strideNode, ok := s.(*Stride)
	if !ok {
		t.Fatalf("empty Stride returned %T, want *Stride", s)
	}
	if strideNode.Rank() != 1 {
		t.Errorf("empty-shape stride rank = %d, want 1", strideNode.Rank())
	}
	// range analysis must not blow up on it either
	AnalyzeRange(s)
	if r, ok := s.Range().(*RangeInterval); !ok || r.UpperBound() != 1 {
		t.Errorf("empty-shape stride range = %v, want interval [0,1)", s.Range())
	}
}

func TestAnalyzeRange(t *testing.T) {
	f := &Factory{}
	v := f.Variable("t", "t", 100)
	AnalyzeRange(v)
	if r, ok := v.Range().(*RangeInterval); !ok || r.UpperBound() != 100 {
		t.Errorf("variable range = %v, want interval [0,100)", v.Range())
	}

	half := f.Binary(Div, v, f.Const(2))
	mod32 := f.Binary(Mod, v, f.Const(32))
	sum := f.Binary(Add, v, v)
	AnalyzeRange(half)
	AnalyzeRange(mod32)
	AnalyzeRange(sum)

	if r, ok := half.Range().(*RangeInterval); !ok || r.UpperBound() != 50 {
		t.Errorf("(t/2) range = %v, want interval [0,50)", half.Range())
	}
	if r, ok := mod32.Range().(*RangeInterval); !ok || r.UpperBound() != 32 {
		t.Errorf("(t%%32) range = %v, want interval [0,32)", mod32.Range())
	}
	if !sum.Range().IsUnknown() {
		t.Errorf("(t+t) range = %v, want unknown", sum.Range())
	}

	// v/3 for v in [0, 10): true range is [0, 4) (ceil, not floor)
	v10 := f.Variable("t", "t10", 10)
	ceil := f.Binary(Div, v10, f.Const(3))
	AnalyzeRange(ceil)
	if r, ok := ceil.Range().(*RangeInterval); !ok || r.UpperBound() != 4 {
		t.Errorf("(t/3) range = %v, want interval [0,4)", ceil.Range())
	}

	// v%10 for v in [0, 5): result is v itself, true range is [0, 5)
	v5 := f.Variable("t", "t5", 5)
	smallMod := f.Binary(Mod, v5, f.Const(10))
	AnalyzeRange(smallMod)
	if r, ok := smallMod.Range().(*RangeInterval); !ok || r.UpperBound() != 5 {
		t.Errorf("(t%%10) range = %v, want interval [0,5)", smallMod.Range())
	}

	// non-positive divider yields unknown
	badDiv := f.Binary(Div, v, f.Const(0))
	AnalyzeRange(badDiv)
	if !badDiv.Range().IsUnknown() {
		t.Errorf("(t/0) range = %v, want unknown", badDiv.Range())
	}

	// const/symbol are unknown
	c := f.Const(3)
	AnalyzeRange(c)
	if !c.Range().IsUnknown() {
		t.Errorf("const range = %v, want unknown", c.Range())
	}
}
