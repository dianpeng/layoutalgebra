package f2

import "testing"

// ---------------------------------------------------------------------------
// Reference CUDA/GPU layout scenarios — end-to-end evidence that the F2
// solver handles normal layout reasoning: hardware coordinate bits -> data
// coordinate mapping -> byte-space stride -> bank analysis -> swizzle, with
// the bijectivity/injectivity invariants certified by exact F2 rank.
// ---------------------------------------------------------------------------

// rankOf certifies the rank of a vector's coefficient matrix over its basis
// variables (all-constant coefficients in these scenarios).
func rankOf(t *testing.T, v Vector) int {
	t.Helper()
	cm, err := v.CoeffMatAll()
	if err != nil {
		t.Fatalf("CoeffMatAll: %v", err)
	}
	res, err := GuassianEliminationConstMat(cm)
	if err != nil {
		t.Fatalf("elimination: %v", err)
	}
	return res.Rank
}

// laneVars builds n lane-bit variables named t0..t{n-1}.
func laneVars(n int) []Expr {
	out := make([]Expr, n)
	for i := 0; i < n; i++ {
		out[i] = NewVariable(varName(i))
	}
	return out
}

func varName(i int) string {
	return "t" + string(rune('0'+i))
}

// TestCudaWarpAccessSaturated: a full warp (5 lane bits) covering an 8x8
// float tile row-major. The composed hw->byte-offset map is bijective, the
// bank window sees all 5 lane bits (rank 5 == bank.Range), so the access is
// conflict-free by construction and the swizzler correctly reports
// saturated — there is nothing to optimize.
func TestCudaWarpAccessSaturated(t *testing.T) {
	lane := laneVars(7) // t0..t6 — 128 threads
	hw := NewVector(lane)

	// data coords: 8x16 float tile (7 offset bits), row-major with two
	// coordinate bits permuted — a pure bit permutation keeps the map bijective
	mapped, err := MapAccess(hw, NewVector([]Expr{
		lane[0], lane[1], lane[2], lane[4], lane[3], lane[5], lane[6],
	}))
	if err != nil {
		t.Fatalf("MapAccess: %v", err)
	}

	// byte space: float = 4 bytes = 2 shift bits
	strided, err := StrideMap([]int{0, 0, 0, 0, 0, 0, 0}, mapped, 2)
	if err != nil {
		t.Fatalf("StrideMap: %v", err)
	}

	// the composed map must be bijective on the tile (square, full rank)
	if r := rankOf(t, strided); r != 7 {
		t.Fatalf("composed map rank = %d, want 7 (bijective on the tile)", r)
	}

	// bank analysis: [2, 7) over the byte vector
	bank := Bank{Skip: 2, Range: 5, VariablePrefix: "t"}
	bankBits := strided.Slice(bank.Skip, bank.Skip+bank.Range)
	if r := rankOf(t, bankBits); r != 5 {
		t.Fatalf("bank rank = %d, want 5 (saturated)", r)
	}

	// conflict-free by construction: the swizzler must say saturated
	if _, err := SwizzleMap(bank, strided); err != SwizzleErrorSaturated {
		t.Fatalf("SwizzleMap err = %v, want SwizzleErrorSaturated", err)
	}
}

// TestCudaStridedAccessSwizzle: a conflicted access pattern. Five lane bits
// feed a 9-bit tile offset where the bank window [2,7) sees only three
// independent lane bits (rank 3 < Range 5: heavy conflicts), while two
// independent lane bits sit beyond the bank range as free degrees. The
// swizzler must XOR them in, saturating the bank (rank 5) while preserving
// injectivity of the composed map — the bijective/injective invariant.
func TestCudaStridedAccessSwizzle(t *testing.T) {
	lane := laneVars(5) // t0..t4
	// 9-bit offset, MSB-first. Bank window d2..d6 spans only {t0,t1,t2};
	// d7=t4, d8=t3 are independent free degrees beyond the bank range.
	offset := NewVector([]Expr{
		lane[0], lane[1], lane[2], lane[1], lane[0],
		lane[2], lane[1], lane[4], lane[3],
	})

	strided, err := StrideMap([]int{0, 0, 0, 0, 0, 0, 0, 0, 0}, offset, 2)
	if err != nil {
		t.Fatalf("StrideMap: %v", err)
	}

	bank := Bank{Skip: 2, Range: 5, VariablePrefix: "t"}

	// pre-swizzle: conflicted — bank rank 3 of max 5
	preBank := strided.Slice(bank.Skip, bank.Skip+bank.Range)
	if r := rankOf(t, preBank); r != 3 {
		t.Fatalf("pre-swizzle bank rank = %d, want 3 (conflicted)", r)
	}

	swizzled, err := SwizzleMap(bank, strided)
	if err != nil {
		t.Fatalf("SwizzleMap: %v", err)
	}

	// post-swizzle: bank saturated (conflicts eliminated)
	postBank := swizzled.Slice(bank.Skip, bank.Skip+bank.Range)
	if r := rankOf(t, postBank); r != 5 {
		t.Fatalf("post-swizzle bank rank = %d, want 5 (saturated)", r)
	}

	// the composed map stays injective (swizzle is a bijection: M^-1 = M)
	if r := rankOf(t, swizzled); r != 5 {
		t.Fatalf("post-swizzle composed rank = %d, want 5 (injectivity preserved)", r)
	}

	// and the result is canonical (fold-stable)
	swizzled.Fold()
	if r := rankOf(t, swizzled); r != 5 {
		t.Fatalf("post-fold rank = %d, want 5", r)
	}
}
