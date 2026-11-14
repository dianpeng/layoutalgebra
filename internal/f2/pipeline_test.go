package f2

import "testing"

// ---------------------------------------------------------------------------
// Layout optimization pipeline tests
// ---------------------------------------------------------------------------

// TestGmemOptimize: the full GMEM pipeline on the perfect-coalescing
// scenario — final offset vector for codegen plus the certified score.
func TestGmemOptimize(t *testing.T) {
	hw := NewVector([]Expr{
		NewVariable("w1"), NewVariable("w0"),
		NewVariable("t4"), NewVariable("t3"), NewVariable("t2"),
		NewVariable("t1"), NewVariable("t0"),
	})
	res, err := GmemOptimize(hw, []GmemAxis{{"i", 2}, {"j", 5}}, 5, 2)
	if err != nil {
		t.Fatalf("GmemOptimize: %v", err)
	}
	if res.Offset.Size() != 7+2 {
		t.Errorf("offset size = %d, want 9 (7 coord bits + 2 dtype bits)", res.Offset.Size())
	}
	if !res.Plan.Perfect {
		t.Errorf("Plan.Perfect = false, want true")
	}
	if res.Score.LineRank != 0 || res.Score.SectorRank != 2 {
		t.Errorf("Score = %+v, want {0, 2}", res.Score)
	}
	// the offset vector is canonical: fold-stable and dumpable for codegen
	res.Offset.Fold()
	if res.Offset.Size() != 9 {
		t.Errorf("offset not fold-stable")
	}
}

// TestSmemOptimizeSaturated: bijective access (permutation mode) whose bank
// slice is already fully ranked — the pipeline returns the strided layout
// unchanged with Swizzled=false.
func TestSmemOptimizeSaturated(t *testing.T) {
	lane := laneVars(7)
	hw := NewVector(lane)
	dataCoord := NewVector([]Expr{
		lane[0], lane[1], lane[2], lane[4], lane[3], lane[5], lane[6],
	})
	res, err := SmemOptimize(hw, dataCoord, 2, Bank{Skip: 2, Range: 5, VariablePrefix: "t"})
	if err != nil {
		t.Fatalf("SmemOptimize: %v", err)
	}
	if res.Swizzled {
		t.Errorf("Swizzled = true, want false (bank already saturated)")
	}
	if res.Offset.Size() != 9 {
		t.Errorf("offset size = %d, want 9", res.Offset.Size())
	}
	if r := rankOf(t, res.Offset.Slice(2, 7)); r != 5 {
		t.Errorf("bank rank = %d, want 5", r)
	}
}

// TestSmemOptimizeSwizzled: rectangular pattern (direct mode) with a
// conflicted bank slice — the pipeline applies the swizzle and the bank rank
// rises from 3 to 5 while injectivity is preserved.
func TestSmemOptimizeSwizzled(t *testing.T) {
	lane := laneVars(5)
	hw := NewVector(lane)
	dataCoord := NewVector([]Expr{
		lane[0], lane[1], lane[2], lane[1], lane[0],
		lane[2], lane[1], lane[4], lane[3],
	})

	res, err := SmemOptimize(hw, dataCoord, 2, Bank{Skip: 2, Range: 5, VariablePrefix: "t"})
	if err != nil {
		t.Fatalf("SmemOptimize: %v", err)
	}
	if !res.Swizzled {
		t.Errorf("Swizzled = false, want true")
	}

	// pre-swizzle bank rank (rebuilt independently): 3, conflicted
	strided, err := StrideMap(make([]int, 9), dataCoord, 2)
	if err != nil {
		t.Fatal(err)
	}
	if r := rankOf(t, strided.Slice(2, 7)); r != 3 {
		t.Fatalf("pre bank rank = %d, want 3", r)
	}
	// post-swizzle: saturated
	if r := rankOf(t, res.Offset.Slice(2, 7)); r != 5 {
		t.Errorf("post bank rank = %d, want 5 (swizzle repaired the conflicts)", r)
	}
	// injectivity preserved across the whole offset space
	if r := rankOf(t, res.Offset); r != 5 {
		t.Errorf("composed rank = %d, want 5", r)
	}
}

// TestSmemOptimizeNoFreeDegree: a bijective pattern whose tile is too small
// to hold free bits beyond the bank window — the conservative error
// propagates instead of silently returning an unoptimized layout.
func TestSmemOptimizeNoFreeDegree(t *testing.T) {
	lane := laneVars(5)
	hw := NewVector(lane)
	dataCoord := NewVector([]Expr{lane[0], lane[1], lane[2], lane[3], lane[4]})
	_, err := SmemOptimize(hw, dataCoord, 2, Bank{Skip: 2, Range: 5, VariablePrefix: "t"})
	if err == nil {
		t.Fatal("want SwizzleErrorNoFreeDegree propagation, got nil")
	}
}

// TestGmemOptimizeLaneNotVariable: lane bits must be bare variables under
// the trailing convention — an expression there is a caller error.
func TestGmemOptimizeLaneNotVariable(t *testing.T) {
	hw := NewVector([]Expr{
		NewVariable("w0"),
		NewBinary(Xor, NewVariable("t1"), NewVariable("t0")), // not a bare variable
	})
	_, err := GmemOptimize(hw, []GmemAxis{{"i", 2}}, 1, 0)
	if err == nil {
		t.Fatal("want lane-not-variable error, got nil")
	}
}
