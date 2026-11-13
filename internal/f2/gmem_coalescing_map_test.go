package f2

import "testing"

// ---------------------------------------------------------------------------
// GMEM coalescing map tests
// ---------------------------------------------------------------------------

func TestSubstitute(t *testing.T) {
	x := NewVariable("x")
	w := NewVariable("w0")
	s := NewSymbol("M")

	// (w0 ^ x) * M with w0 := 0  =>  x * M  =>  canonical (M * x)
	e := NewBinary(Mul, NewBinary(Xor, w, x), s)
	got := Substitute(e, map[string]Expr{"w0": NewConstZero()})
	want := NewMul(s, x)
	if !got.IsEqual(want) {
		t.Errorf("Substitute((w0^x)*M, w0:=0) = %v, want %v", got, want)
	}

	// symbols substitute too (codegen-time concretization)
	got = Substitute(e, map[string]Expr{"M": NewConstOne(), "w0": NewConstZero()})
	if !got.IsEqual(x) {
		t.Errorf("Substitute((w0^x)*M, w0:=0, M:=1) = %v, want x", got)
	}

	// untouched variables stay
	y := NewVariable("y")
	got = Substitute(y, map[string]Expr{"x": NewConstOne()})
	if !got.IsEqual(y) {
		t.Errorf("Substitute(y, x:=1) = %v, want y", got)
	}
}

// laneNameList builds lane basis names t0..t{n-1} matching laneVars.
func laneNameList(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = varName(i)
	}
	return out
}

// TestGmemCoalescingMapPerfect: 7-bit hw coords (2 warp + 5 lane), logical
// space [i:3, j:5], float (dtypeSize 2). The greedy places all 5 lane bits
// in the innermost span (5+2=7 >= 5): perfect. The scored objective hits
// both volume floors: one 128B line, four 32B sectors.
func TestGmemCoalescingMapPerfect(t *testing.T) {
	hw := NewVector([]Expr{
		NewVariable("w1"), NewVariable("w0"), // warp bits
		NewVariable("t4"), NewVariable("t3"), NewVariable("t2"),
		NewVariable("t1"), NewVariable("t0"), // lane bits (trailing)
	})
	plan, err := GmemCoalescingMap(hw, []GmemAxis{{"i", 2}, {"j", 5}}, 5, 2)
	if err != nil {
		t.Fatalf("GmemCoalescingMap: %v", err)
	}
	if !plan.Perfect {
		t.Errorf("Perfect = false, want true")
	}
	if plan.Innermost != "j" || plan.Span != 7 || plan.LaneBitsFit != 5 || plan.LaneBitsSpill != 0 {
		t.Errorf("plan = %+v, want innermost j span 7 fit 5 spill 0", plan)
	}

	mapped := plan.Matrix.Apply(hw)
	strided, err := StrideMap([]int{0, 0, 0, 0, 0, 0, 0}, mapped, 2)
	if err != nil {
		t.Fatalf("StrideMap: %v", err)
	}
	score, err := ScoreCoalescing(strided, laneNameList(5))
	if err != nil {
		t.Fatalf("ScoreCoalescing: %v", err)
	}
	// floor: sectorRank >= 5+2-5 = 2, lineRank >= 0 — both met exactly
	if score.SectorRank != 2 || score.LineRank != 0 {
		t.Errorf("score = %+v, want {LineRank:0 SectorRank:2} (perfect coalescing)", score)
	}
}

// TestGmemCoalescingMapSpill: narrow innermost axis (1 bit + dtype 2 = span
// 3 < 5): three lane bits fit inside the axis, two spill past it. The
// positional objective is still optimal — spill is a layout-semantic report,
// not a positional cost.
func TestGmemCoalescingMapSpill(t *testing.T) {
	hw := NewVector([]Expr{
		NewVariable("w0"),
		NewVariable("t4"), NewVariable("t3"), NewVariable("t2"),
		NewVariable("t1"), NewVariable("t0"),
	})
	plan, err := GmemCoalescingMap(hw, []GmemAxis{{"i", 5}, {"j", 1}}, 5, 2)
	if err != nil {
		t.Fatalf("GmemCoalescingMap: %v", err)
	}
	if plan.Perfect {
		t.Errorf("Perfect = true, want false (span 3 < 5)")
	}
	if plan.Innermost != "j" || plan.Span != 3 || plan.LaneBitsFit != 3 || plan.LaneBitsSpill != 2 {
		t.Errorf("plan = %+v, want innermost j span 3 fit 3 spill 2", plan)
	}

	mapped := plan.Matrix.Apply(hw)
	strided, err := StrideMap([]int{0, 0, 0, 0, 0, 0}, mapped, 2)
	if err != nil {
		t.Fatalf("StrideMap: %v", err)
	}
	score, err := ScoreCoalescing(strided, laneNameList(5))
	if err != nil {
		t.Fatalf("ScoreCoalescing: %v", err)
	}
	if score.SectorRank != 2 || score.LineRank != 0 {
		t.Errorf("score = %+v, want {0, 2} — greedy positional optimum despite axis spill", score)
	}
}

// TestScoreCoalescingStrided: the conflicted pattern from
// TestCudaStridedAccessSwizzle scores badly on the GMEM objective, and the
// SMEM swizzle — while saturating the bank objective — does NOT improve
// coalescing. SMEM bank optimization and GMEM coalescing are independent
// axes; this locks that the scorer sees the difference.
func TestScoreCoalescingStrided(t *testing.T) {
	lane := laneVars(5)
	offset := NewVector([]Expr{
		lane[0], lane[1], lane[2], lane[1], lane[0],
		lane[2], lane[1], lane[4], lane[3],
	})
	strided, err := StrideMap([]int{0, 0, 0, 0, 0, 0, 0, 0, 0}, offset, 2)
	if err != nil {
		t.Fatalf("StrideMap: %v", err)
	}

	pre, err := ScoreCoalescing(strided, laneNameList(5))
	if err != nil {
		t.Fatalf("ScoreCoalescing pre: %v", err)
	}
	// sector rows [0, 11-5): [t0,t1,t2,t1,t0,t2] -> rank 3; line rows [0, 4): rank 3
	if pre.SectorRank != 3 || pre.LineRank != 3 {
		t.Errorf("pre score = %+v, want {3, 3} (conflicted: 8 lines, 8 sectors)", pre)
	}

	swizzled, err := SwizzleMap(Bank{Skip: 2, Range: 5, VariablePrefix: "t"}, strided)
	if err != nil {
		t.Fatalf("SwizzleMap: %v", err)
	}
	post, err := ScoreCoalescing(swizzled, laneNameList(5))
	if err != nil {
		t.Fatalf("ScoreCoalescing post: %v", err)
	}
	// the swizzle injects t4/t3 into byte slots 5/6: sector rank rises to 4
	if post.SectorRank != 4 {
		t.Errorf("post score = %+v, want SectorRank 4 (swizzle trades coalescing for banks)", post)
	}
}

// TestGmemCoalescingMapValidation: bad inputs must error, not panic.
func TestGmemCoalescingMapValidation(t *testing.T) {
	hw := NewVector([]Expr{NewVariable("t0"), NewVariable("t1")})

	if _, err := GmemCoalescingMap(hw, nil, 1, 0); err == nil {
		t.Error("no axes: want error")
	}
	if _, err := GmemCoalescingMap(hw, []GmemAxis{{"i", 3}}, 1, 0); err == nil {
		t.Error("axes total != hw size: want error")
	}
	if _, err := GmemCoalescingMap(hw, []GmemAxis{{"i", 1}, {"j", 1}}, 0, 0); err == nil {
		t.Error("laneBits 0: want error")
	}
	if _, err := GmemCoalescingMap(hw, []GmemAxis{{"i", 1}, {"j", 1}}, 3, 0); err == nil {
		t.Error("laneBits > size: want error")
	}
	if _, err := GmemCoalescingMap(hw, []GmemAxis{{"i", 1}, {"j", 0}}, 1, 0); err == nil {
		t.Error("zero-bit axis: want error")
	}
	if _, err := GmemCoalescingMap(hw, []GmemAxis{{"i", 1}, {"j", 1}}, 1, -1); err == nil {
		t.Error("negative dtypeSize: want error")
	}
}
