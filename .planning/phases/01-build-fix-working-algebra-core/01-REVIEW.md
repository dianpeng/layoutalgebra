---
phase: 01-build-fix-working-algebra-core
reviewed: 2026-11-12T13:47:07Z
depth: standard
files_reviewed: 14
files_reviewed_list:
  - internal/check/check.go
  - internal/f2/access_map.go
  - internal/f2/coeff_mat.go
  - internal/f2/coeff_mat_test.go
  - internal/f2/expr.go
  - internal/f2/expr_test.go
  - internal/f2/fold.go
  - internal/f2/fold_test.go
  - internal/f2/matop.go
  - internal/f2/stride_map.go
  - internal/f2/swizzle_map.go
  - internal/f2/vector.go
  - internal/f2/vector_test.go
  - internal/f2/visit.go
findings:
  critical: 7
  warning: 15
  info: 12
  total: 34
status: issues_found
---

# Phase 01: Code Review Report

**Reviewed:** 2026-11-12T13:47:07Z
**Depth:** standard
**Files Reviewed:** 14
**Status:** issues_found

## Summary

The Fold engine, F2 scalar ops, Gather/Slice, and the Apply map/reduce roles are
correct and well-tested by the existing suite (all tests pass, `go vet` clean).
However, the entire matrix/elimination/swizzle path — `CoeffMat.ToConst`,
`GuassianEliminationConstMat`, `Vector.CoeffMatAll` /
`extractLinearCoeffVector`, `Vector.ShiftL`/`ShiftR`, and `SwizzleMap` — is
broken. Every headline finding below was empirically confirmed by exercising the
code in a scratch copy of the module (repo left untouched): the basic public
entry points `CoeffMatAll`, `GuassianEliminationConstMat`, `StrideMap`, and
`SwizzleMap` panic or return wrong results on their most ordinary inputs. The
existing test files only cover the fold/scalar/gather layer, which is why the
suite is green despite this.

## Critical Issues

### CR-01: `CoeffMat.ToConst` never appends rows — returned matrix is always empty

**File:** `internal/f2/coeff_mat.go:105-118`
**Issue:** The inner loop builds `outRow` but `vv = append(vv, outRow)` is
missing, so `vv` (length 0, capacity m) is returned. Verified: `ToConst` on a
2x2 identity returns `raw=[]` with `err=nil`. Downstream,
`GuassianEliminationConstMat` does `raw[cursorI]` on the empty slice and panics
with `index out of range [0] with length 0` (matop.go:98) for **any** input.
This takes out `RankConstMat`, `swizzler.vectorInfo`, and `SwizzleMap` entirely.
**Fix:**
```go
for _, row := range self.v {
  outRow := make([]F2, 0, self.n)
  for _, ele := range row {
    if ele.Kind() == Const {
      outRow = append(outRow, ele.(*NodeConst).Value())
    } else {
      return ConstMat{}, fmt.Errorf("not a constant matrix")
    }
  }
  vv = append(vv, outRow) // MISSING — add this
}
```

### CR-02: Gaussian elimination rank formula is inverted — identity matrix reports rank 0

**File:** `internal/f2/matop.go:133-137`
**Issue:** After the elimination loop, `cursorI` equals the number of pivots
found, which *is* the rank. The code instead computes
`rank = min(m - cursorI, n - cursorJ)` — the dimension of the *remaining*
submatrix. Verified (with CR-01 patched): the 2x2 identity reports `rank=0`;
`basisIdx` is consequently always empty. `EliminationResult.Rank` drives the
`bankRank == bank.Range` saturation check in swizzle_map.go, so every swizzle
decision made from it is wrong.
**Fix:**
```go
rank := cursorI
basisIdx := permIdx[0:rank]
```

### CR-03: Stale pivot row after row swap — elimination subtracts the wrong row

**File:** `internal/f2/matop.go:96-131`
**Issue:** `lhsRow := raw[cursorI]` is captured *before* the pivot search. When
the pivot is zero and `swapRow(cursorI, j)` fires, `raw[cursorI]` now holds the
swapped-up row, but `lhsRow` still points at the old row (which has 0 at the
pivot column). The elimination loop then subtracts a row with a zero pivot,
clearing nothing. Verified (with CR-01/CR-02 patched): for
`[[0,1,1],[1,0,1],[1,1,0]]` (true F2 rank 2 — rows sum to zero), the buggy code
reports rank 3 and emits `row0 == row2` in the echelon form.
**Fix:** Re-read the pivot row after the swap (or capture it after step (4)):
```go
if pivot == Zero {
  for j := cursorI + 1; j < m; j++ {
    if raw[j][cursorJ] != Zero {
      swapRow(cursorI, j)
      break
    }
  }
}
if pivot == Zero { cursorJ++; continue }
lhsRow = raw[cursorI] // re-read after potential swap
```

### CR-04: `extractLinearCoeffVector` panics on a bare-variable closure

**File:** `internal/f2/vector.go:419-424`
**Issue:** For a closure that is just the variable itself (coefficient 1),
`check.Check(c.Kind() == Binary)` fails and panics. Verified:
`CoeffMatAll()` on `NewVector([x, y])` — the most basic input, which should
yield the 2x2 identity coefficient matrix — panics at vector.go:422. Any swizzle
input built from plain bit variables hits this.
**Fix:** Handle the coefficient-1 case before the Binary assertion:
```go
c := exprMap[vName]
if c == nil {
  out = append(out, NewConstZero())
} else if c.Kind() == Variable { // bare variable => coefficient One
  check.Check(c.(*NodeVariable).Name() == vName)
  out = append(out, NewConstOne())
} else {
  // existing binary chase
}
```

### CR-05: Coefficient extraction expects the wrong tree shape — `S*x` panics, `S1*(S2*x)` silently drops `S2`

**File:** `internal/f2/vector.go:425-440`
**Issue:** The code asserts `cR.Kind() == Binary` and takes `cR.(*NodeBinary).R()`
as the variable, then returns `cL` as the coefficient. But the canonical folded
form produced by this very fold engine is `S*x` (static on the left, per
rewriteCommute), whose `cR` *is* the variable — verified: `CoeffMatAll()` on
`[S*x]` panics at vector.go:434. Conversely, for the shape the check accepts,
`cL * (inner * x)`, the inner coefficient factor is silently discarded
(coefficient returned as `cL` only, dropping `inner`). The comment says "chasing
the expression to collect all LHS symbols until the inner most RHS symbol", but
no chase loop exists.
**Fix:** Walk the right spine of the closure, multiplying every left factor:
```go
coeff := Expr(NewConstOne())
cur := c
for cur.Kind() == Binary && cur.(*NodeBinary).Op() == Mul {
  b := cur.(*NodeBinary)
  if b.R().Kind() == Variable {
    check.Check(b.R().(*NodeVariable).Name() == vName)
    coeff = NewBinary(Mul, coeff, b.L())
    break
  }
  coeff = NewBinary(Mul, coeff, b.L())
  cur = b.R()
}
out = append(out, Fold(coeff))
```

### CR-06: `Vector.ShiftL` position math is wrong — shifts by 1 regardless of `k`, panics at `k=0`, leaves nil holes

**File:** `internal/f2/vector.go:90-107`
**Issue:** With `overflow=true`, `len(vec) = L + k`, and the loop computes
`pos = len(vec) - k - idx = i + 1` — every element moves by exactly one
position no matter what `k` is. Verified: `ShiftL(2, true)` on a 2-element
vector produces size 4 with `vec[0] = nil`, `vec[3] = nil` (only positions
1,2 written). `ShiftL(0, true)` panics with `index out of range [2] with
length 2` (verified). `StrideMap` calls `ShiftL(dtypeSize, true)` directly, so
`dtypeSize=0` panics and any other value produces a vector containing nil
`Expr` entries that will nil-dereference on the first `.Kind()` call
downstream. The correct little-endian widening shift is `vec[i+k] = self.vec[i]`.
**Fix:**
```go
for i := 0; i < len(self.vec); i++ {
  vec[i+k] = self.vec[i]   // overflow=true; remaining low k slots filled with NewConstZero()
}
```
(and fill vacated positions with `NewConstZero()`, see WR-02; for
`overflow=false` write into a length-L slice with bounds guard).

### CR-07: Swizzle indexes the free vector with an absolute input-space index

**File:** `internal/f2/swizzle_map.go:199-202, 225`
**Issue:** `freeBasisIdx` is deliberately converted to absolute input-space
indices (`freeBasisIdx = append(freeBasisIdx, freeIdx[bIdx])`), but line 225
then does `freeVector.Index(freeBasisIdx[idx])` — `freeVector` only has
`len(freeIdx)` elements and is indexed 0..len(freeIdx)-1. Since `freeIdx`
starts at `bank.Range + 1 >= 2`, every basis index is out of range for
`freeVector`: the first swizzle XOR panics (or, if the free vector happened to
be longer, would pick the wrong expression). Reachable as soon as CR-01/02/04/05
are fixed; confirmed by inspection (the path is currently masked by the earlier
panics).
**Fix:** Keep basis indices in free-vector space for indexing `freeVector`:
```go
swizzleValue := NewBinary(Xor, existedValue, freeVector.Index(result.BasisIndex[idx]))
```
(or index `input` with `freeBasisIdx[idx]` instead — both identify the same bit).

## Warnings

### WR-01: `doCoeffMat` never folds the symbolized expression — duplicate same-variable closures silently overwrite

**File:** `internal/f2/vector.go:231-254, 384`
**Issue:** `extractLinearCoeffVector`'s forward pass stores one closure per
variable name in `exprMap[name] = e` — a second closure for the same variable
silently *overwrites* the first. For an un-folded input like
`(M*x) + (N*x)`, the coefficient of `x` loses the `M` term with no error. The
function's own doc comment (step 5) assumes normalized input, but
`doCoeffMat` calls it directly after `SymbolizeExcept` without `Fold`.
**Fix:** Fold each symbolized expression before extraction:
`tVec = append(tVec, Fold(SymbolizeExcept(expr, basis)))`, and/or merge
same-name closures with `NewBinary(Add, old, new)` + `Fold` instead of
overwriting.

### WR-02: `ShiftR` (and the fixed `ShiftL`) leave vacated positions as nil `Expr` instead of `ConstZero`

**File:** `internal/f2/vector.go:90-126`
**Issue:** Both shift functions `make` the destination slice and only write
shifted positions; vacated slots stay nil. Any consumer calling `.Kind()` /
`.IsEqual()` on those entries nil-panics. The convention elsewhere (e.g.
`ShiftMatrix`'s "bit left will be zeroed") is zero-fill.
**Fix:** Initialize vacated slots with `NewConstZero()`:
`for i := range vec { vec[i] = NewConstZero() }` before the copy loop.

### WR-03: `IsPerm` does not check column uniqueness

**File:** `internal/f2/coeff_mat.go:127-143`
**Issue:** Only "each row contains exactly one 1" is verified.
`[[1,0],[1,0]]` passes `IsPerm` but is not a permutation matrix. Column
uniqueness (exactly one 1 per column) must also hold.
**Fix:** Track the set of columns that already have a 1 and return false on
collision.

### WR-04: `IsIdentity` panics on non-square matrices

**File:** `internal/f2/coeff_mat.go:145-165`
**Issue:** `self.v[i][i]` is evaluated for `i` up to `m-1`; when `n < m` this
indexes past the row end and panics instead of returning false.
**Fix:** Add `if self.m != self.n { return false }` at the top.

### WR-05: Swizzle free-bit scan starts at `bank.Range + 1` instead of `bank.Skip + bank.Range`

**File:** `internal/f2/swizzle_map.go:178`
**Issue:** Bank bits occupy `[Skip, Skip+Range)`. For the documented CUDA
config `(Skip=2, Range=5)` the scan starts at index 6 — inside the bank range —
so a bank bit is eligible as a "free" XOR source, contrary to the algorithm
description. Should be `for i := bank.Skip + bank.Range; i < input.Size(); i++`.

### WR-06: Empty free vector panics instead of returning `SwizzleErrorNoFreeDegree`

**File:** `internal/f2/swizzle_map.go:188-198`, `internal/f2/coeff_mat.go:41`
**Issue:** When no free dimensions exist, `freeVector` is 0-sized;
`vectorInfo` → `CoeffMatAll` → `NewCoeffMat(empty)` hits `check.Check(m > 0)`
and panics. The `if result.Rank == 0` guard that returns
`SwizzleErrorNoFreeDegree` is unreachable for the empty case. Return the error
before calling `vectorInfo` when `len(freeIdx) == 0`.

### WR-07: `input.Slice(bank.Skip, bank.Skip+bank.Range)` is unchecked

**File:** `internal/f2/swizzle_map.go:131`
**Issue:** If `Skip+Range > input.Size()` this is a runtime slice-bounds panic.
Validate up front and return an error (or clamp) — the function already has an
error-returning API.

### WR-08: `Vector.Add` has no size check

**File:** `internal/f2/vector.go:55-70`
**Issue:** The loop iterates `that.vec` and indexes `self.vec[idx]`: if `self`
is shorter it panics; if `that` is shorter the result is silently truncated to
`that`'s length. Add `check.Check(self.Size() == that.Size())` (or return an
error) before the loop.

### WR-09: `PermutationMatrix` panics on rank mismatch and misses non-bijective inputs

**File:** `internal/f2/matop.go:11-33`
**Issue:** (a) `check.Check(lhs.Rank() == rhs.Rank())` panics even though the
function returns `error` — inconsistent with its own contract. (b) Only the
rhs→lhs direction is matched: `rhs=[a,a]`, `lhs=[a,b]` produces
`out[0][0]=out[1][0]=1` (two 1s in one column) with no error, and the result is
not a permutation. Track matched lhs columns and error on reuse or unmatched
lhs entries.

### WR-10: Fold driver discards errors and silently gives up at the iteration cap

**File:** `internal/f2/fold.go:404-419`
**Issue:** (a) `doRun` ignores the error returned by `self.rewrite(expr)` —
today no rule returns an error, but any future rule error would be swallowed.
(b) If `maxBound` (10000) is exhausted without reaching a fixpoint, `run`
returns the last expression with no indication that canonical form was not
reached — a silent wrong-answer path for a normalization engine whose entire
purpose is canonical form. Surface both (return error / panic via `check.Bug`).

### WR-11: `NewFromSemanticMapping` indexes `namePrefix[dim]` without a length check

**File:** `internal/f2/vector.go:521-539`
**Issue:** If `len(namePrefix) < len(dims)` this panics with index out of
range. `NewFromHardware` right above has `check.Check(len(bits) ==
len(namePrefix))`; this constructor needs the same guard.

### WR-12: `StrideMap` does not validate `dtypeSize`; `tileShape` is checked but unused

**File:** `internal/f2/stride_map.go:18-25`
**Issue:** The comment says `dtypeSize` "must be 0/1/2" but nothing enforces it;
combined with CR-06, `dtypeSize=0` panics and other invalid values produce
garbage. Also `tileShape` is length-checked against the input and then never
used, and the function never returns a non-nil error. Validate `dtypeSize`
(0..2, or `common.IsPowerOf2`-style bound) and return an error on violation.

### WR-13: `NameBelong` prefix matching is fragile across variable families

**File:** `internal/f2/swizzle_map.go:40-42`
**Issue:** `strings.HasPrefix` means a bank prefix `thr1` also matches
variables from family `thr10` (names are built as `<prefix><dim>_<idx>`, e.g.
`thr10_0`). A boundary-aware match (prefix followed by digit/underscore
boundary, or an exact family registry) avoids silently mixing families.

### WR-14: `check.fail` asserts `a[0].(string)` unchecked

**File:** `internal/check/check.go:15-19`
**Issue:** If the first arg is not a string (or a format string is passed with
missing args), `fail` itself panics with a confusing secondary error (or emits
`%!v(MISSING)` garbage in the message). Use the comma-ok form and fall back to
`fmt.Sprint(a...)`.

### WR-15: `ShiftMatrix` panics on negative `k`

**File:** `internal/f2/matop.go:158-173`
**Issue:** Only `k >= size` is guarded; `k < 0` makes `raw[row][k]` index
negative and panic. Validate `k >= 0` (or document+check).

## Info

### IN-01: Dead consume-once logic in `Replace`

**File:** `internal/f2/expr.go:116-120` — `replacement.Old = nil` mutates the
range-loop copy, so the intended "each replacement fires once" semantics never
happens; if two replacements share an `Old`, the last silently wins. Delete the
line or index into the slice.

### IN-02: Inconsistent mutate-and-return protocol between fold rules

**File:** `internal/f2/fold.go:124-131` vs `310-315` — `rewriteCommute` case #1
mutates via `Commute()` and returns `bin` (self-replacement), while
`rewriteSymbol` case #1 mutates and returns `nil`. Both currently work only
because of rule ordering; a future reorder can double-apply or skip rules.
Pick one protocol (prefer pure: return a new node).

### IN-03: `Matrix` interface is declared but never used

**File:** `internal/f2/coeff_mat.go:9-13` — nothing accepts or returns the
`Matrix` interface; all code uses `CoeffMat`/`ConstMat` concretely. Remove or
start using it.

### IN-04: `swizzleVectorInfo.U` is written but never read

**File:** `internal/f2/swizzle_map.go:34, 77` — `U: result.Matrix` is stored;
no consumer reads `U`. Dead field.

### IN-05: `Vector.Extent` duplicates `Vector.Index`

**File:** `internal/f2/vector.go:25, 35` — identical implementations; keep one.

### IN-06: `dcheckEnabled` is never set by any build-tag file

**File:** `internal/check/check.go:12-13` — the comment references build tags,
but no `dcheck_enabled.go` exists in the package; `DCheck` is permanently
disabled dead code. Add the tagged files or remove `DCheck`.

### IN-07: `NodeConst.Int()` duplicates `F2.Int()`

**File:** `internal/f2/expr.go:150-155` — same body as `F2.Int()` (expr.go:134);
delegate or drop.

### IN-08: No-op `break` statements in fold driver

**File:** `internal/f2/fold.go:396, 398` — `break` inside a `switch` breaks the
switch, not anything else; both are dead. Remove.

### IN-09: `NewCoeffMat`/`NewConstMat` alias the caller's input slices

**File:** `internal/f2/coeff_mat.go:39-57` — the input `[][]Expr`/`[][]F2` is
stored directly; later mutation of the caller's slice mutates the matrix (and
`GuassianEliminationConstMat` mutates `Raw()` in place). Copy defensively or
document the aliasing.

### IN-10: `orderExpr` returns nil silently for unknown order

**File:** `internal/f2/visit.go:59-68` — `VisitBy` over a nil list is a silent
no-op; `check.Bug` on the default branch would catch misuse.

### IN-11: `CoeffMat.Apply` panics on a 0-column matrix

**File:** `internal/f2/coeff_mat.go:178-199` — `cur := m[0]` on an empty `m`
(when `n == 0`) panics; guard or document the non-empty precondition.

### IN-12: Inconsistent `fail` kind strings in check package

**File:** `internal/check/check.go:105` — `CheckInBounds` uses kind
`"out of bounds"` while everything else uses `CHECK*`/`DCHECK`/`BUG`/`UNREACHABLE`
prefixes; downstream log parsing (if any) must special-case it.

---

_Reviewed: 2026-11-12T13:47:07Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
