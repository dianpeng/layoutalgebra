package f2

import (
  "reflect"
  "testing"
)

// End-to-end layout-pipeline tests: hardware coordinate bits -> MapAccess ->
// StrideMap -> CoeffMatAll -> Gaussian elimination/rank -> SwizzleMap. The
// headline property: the final vector's per-bit expressions are exact,
// Fold-idempotent trees directly interpretable into index arithmetic.

// ---------------------------------------------------------------------------
// A: access map (permutation matrix) — MAP-01
//
// Probe-verified: PermutationMatrix marks out[i][j] where rhs[i] == lhs[j],
// building A such that A·lhs = rhs — the INVERSE of the old doc comment
// "A * rhs = lhs" (recorded as a finding in SUMMARY.md; behavior is locked
// here, not changed).
// ---------------------------------------------------------------------------

func TestPipelineAccessMap(t *testing.T) {
  hw := NewFromHardware([]int{1, 2}, []string{"w", "l"})
  if hw.Size() != 3 {
    t.Fatalf("hw size = %d, want 3 ([w_0, l_0, l_1])", hw.Size())
  }
  w0 := NewVariable("w_0")
  l0 := NewVariable("l_0")
  l1 := NewVariable("l_1")

  dataCoord := NewVector([]Expr{l0, l1, w0})
  out, err := MapAccess(hw, dataCoord)
  if err != nil {
    t.Fatalf("MapAccess error: %v", err)
  }
  if out.Size() != 3 {
    t.Fatalf("MapAccess size = %d, want 3", out.Size())
  }
  if !out.Index(0).IsEqual(l1) {
    t.Errorf("bit 0 = %v, want l_1", out.Index(0))
  }
  if !out.Index(1).IsEqual(w0) {
    t.Errorf("bit 1 = %v, want w_0", out.Index(1))
  }
  if !out.Index(2).IsEqual(l0) {
    t.Errorf("bit 2 = %v, want l_0", out.Index(2))
  }

  // WR-09 lock through the public API: differing bit sets must error
  if _, err := MapAccess(hw, NewVector([]Expr{NewVariable("x_0"), l1, w0})); err == nil {
    t.Error("MapAccess on differing bit sets: err = nil, want non-nil")
  }
}

// ---------------------------------------------------------------------------
// B: stride map — MAP-03 + WR-02 lock (ShiftL zero-fills vacated slots)
// ---------------------------------------------------------------------------

func TestPipelineStrideMap(t *testing.T) {
  w0 := NewVariable("w_0")
  l0 := NewVariable("l_0")
  l1 := NewVariable("l_1")
  vecFromA := NewVector([]Expr{l1, w0, l0}) // [l_1, w_0, l_0]
  tileShape := []int{0, 0, 0}

  // dtypeSize 1: one zero byte-offset bit appended at the LSB end
  out, err := StrideMap(tileShape, vecFromA, 1)
  if err != nil {
    t.Fatalf("StrideMap(dtypeSize=1) error: %v", err)
  }
  if out.Size() != 4 {
    t.Fatalf("StrideMap(dtypeSize=1) size = %d, want 4", out.Size())
  }
  if !out.Index(0).IsEqual(l1) || !out.Index(1).IsEqual(w0) || !out.Index(2).IsEqual(l0) {
    t.Errorf("StrideMap(dtypeSize=1) = [%v %v %v %v], want [l_1 w_0 l_0 0]",
      out.Index(0), out.Index(1), out.Index(2), out.Index(3))
  }
  mustConst(t, out.Index(3), Zero) // WR-02: ConstZero, never a nil Expr hole

  // dtypeSize 0: size unchanged
  same, err := StrideMap(tileShape, vecFromA, 0)
  if err != nil {
    t.Fatalf("StrideMap(dtypeSize=0) error: %v", err)
  }
  if same.Size() != 3 {
    t.Fatalf("StrideMap(dtypeSize=0) size = %d, want 3", same.Size())
  }
  for i := 0; i < 3; i++ {
    if !same.Index(i).IsEqual(vecFromA.Index(i)) {
      t.Errorf("StrideMap(dtypeSize=0) bit %d = %v, want %v", i, same.Index(i), vecFromA.Index(i))
    }
  }

  // dtypeSize 3: invalid, must error (WR-12 lock)
  if _, err := StrideMap(tileShape, vecFromA, 3); err == nil {
    t.Error("StrideMap(dtypeSize=3) err = nil, want non-nil")
  }
}

// ---------------------------------------------------------------------------
// C: swizzle — MAP-02 (+ WR-05/06 locks)
// ---------------------------------------------------------------------------

func TestPipelineSwizzle(t *testing.T) {
  l0 := NewVariable("l_0")
  l1 := NewVariable("l_1")
  w0 := NewVariable("w_0")

  // bank bits [l_0, l_0]: rank 1 of max 2 -> nullspace = {1}; index 2 (l_1)
  // is the free degree; slot 1 becomes l_0 ^ l_1 (existed value on the left)
  input := NewVector([]Expr{l0, l0, l1, w0})
  bank := Bank{Skip: 0, Range: 2, VariablePrefix: "l"}

  out, err := SwizzleMap(bank, input)
  if err != nil {
    t.Fatalf("SwizzleMap error: %v", err)
  }
  if out.Size() != 4 {
    t.Fatalf("output size = %d, want 4", out.Size())
  }
  if !out.Index(0).IsEqual(l0) {
    t.Errorf("slot 0 = %v, want l_0 (unchanged)", out.Index(0))
  }
  want := NewBinary(Xor, l0, l1)
  if !out.Index(1).IsEqual(want) {
    t.Errorf("slot 1 = %v, want %v", out.Index(1), want)
  }
  if !out.Index(2).IsEqual(l1) {
    t.Errorf("slot 2 = %v, want l_1 (unchanged)", out.Index(2))
  }
  if !out.Index(3).IsEqual(w0) {
    t.Errorf("slot 3 = %v, want w_0 (unchanged)", out.Index(3))
  }

  // WR-06 lock: family-mismatched tail -> no free degree -> error, no panic
  mismatched := NewVector([]Expr{l0, l0, w0})
  if _, err := SwizzleMap(bank, mismatched); err != SwizzleErrorNoFreeDegree {
    t.Errorf("family-mismatched tail: err = %v, want SwizzleErrorNoFreeDegree", err)
  }
}

// ---------------------------------------------------------------------------
// D: coefficient matrix + Gaussian elimination — ALG-04
// ---------------------------------------------------------------------------

func TestPipelineCoeffMat(t *testing.T) {
  l0 := NewVariable("l_0")
  l1 := NewVariable("l_1")

  // const-coefficient case: [[1,0],[1,1]], full rank
  v := NewVector([]Expr{l0, NewBinary(Add, l0, l1)})
  mat, err := v.CoeffMatAll()
  if err != nil {
    t.Fatalf("CoeffMatAll error: %v", err)
  }
  m, n := mat.Size()
  if m != 2 || n != 2 {
    t.Fatalf("size = %dx%d, want 2x2", m, n)
  }
  mustConst(t, mat.Index(0, 0), One)
  mustConst(t, mat.Index(0, 1), Zero)
  mustConst(t, mat.Index(1, 0), One)
  mustConst(t, mat.Index(1, 1), One)

  res, err := GuassianEliminationConstMat(mat)
  if err != nil {
    t.Fatalf("elimination error: %v", err)
  }
  if res.Rank != 2 {
    t.Errorf("rank = %d, want 2", res.Rank)
  }

  // rank-deficient case: duplicate rows -> rank 1, basis [0]
  dep := NewVector([]Expr{
    NewBinary(Add, l0, l1),
    NewBinary(Add, l0, l1),
  })
  depMat, err := dep.CoeffMatAll()
  if err != nil {
    t.Fatalf("CoeffMatAll(dependent) error: %v", err)
  }
  depRes, err := GuassianEliminationConstMat(depMat)
  if err != nil {
    t.Fatalf("elimination(dependent) error: %v", err)
  }
  if depRes.Rank != 1 {
    t.Errorf("dependent rank = %d, want 1", depRes.Rank)
  }
  if !reflect.DeepEqual(depRes.BasisIndex, []int{0}) {
    t.Errorf("BasisIndex = %v, want [0]", depRes.BasisIndex)
  }

  // symbolic-coefficient case: (M*l_0) + l_1 -> row [M, 1]
  sym := NewSymbol("M")
  symVec := NewVector([]Expr{
    NewBinary(Add, NewBinary(Mul, sym, l0), l1),
  })
  symMat, err := symVec.CoeffMatAll()
  if err != nil {
    t.Fatalf("CoeffMatAll(symbolic) error: %v", err)
  }
  sm, sn := symMat.Size()
  if sm != 1 || sn != 2 {
    t.Fatalf("symbolic size = %dx%d, want 1x2", sm, sn)
  }
  if !symMat.Index(0, 0).IsEqual(sym) {
    t.Errorf("symbolic coeff (0,0) = %v, want M", symMat.Index(0, 0))
  }
  mustConst(t, symMat.Index(0, 1), One)
}

// ---------------------------------------------------------------------------
// E: composed pipeline — the codegen-readiness headline test
//
// hardware bits -> MapAccess -> StrideMap(dtypeSize 0) -> SwizzleMap. The
// final vector's per-bit expressions must be exact trees, and every bit must
// be Fold-idempotent (already canonical, directly interpretable into index
// arithmetic).
// ---------------------------------------------------------------------------

func TestPipelineCodegen(t *testing.T) {
  l0 := NewVariable("l_0")
  l1 := NewVariable("l_1")
  l2 := NewVariable("l_2")

  // 4-bit hardware vector in [l_0, l_0, l_2, l_1] order; the access map
  // reorders it into data-coord order [l_0, l_0, l_1, l_2] (the duplicated
  // l_0 claims the second equal lhs column)
  hw := NewVector([]Expr{l0, l0, l2, l1})
  mapped, err := MapAccess(hw, NewVector([]Expr{l0, l0, l1, l2}))
  if err != nil {
    t.Fatalf("MapAccess error: %v", err)
  }
  for i, w := range []Expr{l0, l0, l1, l2} {
    if !mapped.Index(i).IsEqual(w) {
      t.Errorf("mapped bit %d = %v, want %v", i, mapped.Index(i), w)
    }
  }

  strided, err := StrideMap([]int{0, 0, 0, 0}, mapped, 0)
  if err != nil {
    t.Fatalf("StrideMap error: %v", err)
  }
  if strided.Size() != 4 {
    t.Fatalf("strided size = %d, want 4", strided.Size())
  }

  // bank bits [l_0, l_0]: nullspace {1}; free degree l_1 at index 2
  final, err := SwizzleMap(
    Bank{Skip: 0, Range: 2, VariablePrefix: "l"},
    strided,
  )
  if err != nil {
    t.Fatalf("SwizzleMap error: %v", err)
  }
  if final.Size() != 4 {
    t.Fatalf("final size = %d, want 4", final.Size())
  }

  // exact per-bit trees on the final vector
  want := []Expr{
    l0,
    NewBinary(Xor, l0, l1),
    l1,
    l2,
  }
  for i, w := range want {
    if !final.Index(i).IsEqual(w) {
      t.Errorf("final bit %d = %v, want %v", i, final.Index(i), w)
    }
  }

  // canonical form: every bit is a fixed point of Fold — this is the property
  // that makes the vector directly interpretable into index arithmetic
  for i := 0; i < final.Size(); i++ {
    if folded := Fold(final.Index(i)); !folded.IsEqual(final.Index(i)) {
      t.Errorf("final bit %d is not Fold-idempotent: Fold(%v) = %v", i, final.Index(i), folded)
    }
  }
}
