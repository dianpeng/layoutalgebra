---
phase: 01-build-fix-working-algebra-core
plan: 01
subsystem: f2-algebra-core
tags: [build-fix, f2-algebra, go]
requires:
  - ALG-05
provides:
  - "Compilable internal/f2 package: expression tree (expr.go), traversal (visit.go), fold engine (fold.go), coefficient matrix (coeff_mat.go), vector (vector.go), matrix ops (matop.go), layout maps (access_map.go, stride_map.go, swizzle_map.go)"
  - "New helpers: Vector.Slice, Vector.Set, GetExprVariableNameSet, NameBelong, BelongVariableNameFamily"
affects:
  - "Plan 01-02 (semantic bug fixes + tests) now has a compiling baseline"
tech-stack:
  added: []
  patterns:
    - "F2 defined type (underlying bool) carrying scalar method block — changed from alias type F2 = bool, which cannot carry methods in Go"
key-files:
  created:
    - internal/f2/expr.go
    - internal/f2/visit.go
    - internal/f2/fold.go
    - internal/f2/coeff_mat.go
    - internal/f2/vector.go
    - internal/f2/matop.go
    - internal/f2/access_map.go
    - internal/f2/stride_map.go
    - internal/f2/swizzle_map.go
  modified:
    - internal/check/check.go
decisions:
  - "F2 changed from type alias (type F2 = bool) to defined type (type F2 bool) — Go forbids methods on an alias of a non-local type; the plan-mandated F2 scalar method block (IsZero/IsOne/Int/Add/Sub/Mul) is otherwise impossible"
  - "BelongVariableNameFamily implemented as a package-level function instead of a method on the Expr interface — Go forbids methods with interface receiver types"
metrics:
  duration: ~25 min
  completed: 2026-11-12
---

# Phase 01 Plan 01: Build-Fix Working Algebra Core Summary

Repaired all nine files of `internal/f2` plus the `internal/check` import surface so the
whole Go module compiles and vets clean: `go build ./...` exit 0, `go vet ./...` exit 0,
`go.mod` byte-identical (zero new dependencies).

## Task Commits

| Task | Commit  | Files                                                        |
|------|---------|--------------------------------------------------------------|
| 1    | e24bc5b | internal/f2/expr.go, internal/f2/visit.go                    |
| 2    | b31e9f4 | fold.go, coeff_mat.go, vector.go, matop.go (+expr.go follow-up) |
| 3    | 5d50e39 | access_map.go, stride_map.go, swizzle_map.go, vector.go      |
| supp | bff9978 | internal/check/check.go (brought under version control)      |

## Repaired Sites Per File

### internal/f2/expr.go
- `NodeBinary.Commute`: added missing empty parameter list.
- `NodeSymbol.IsEqual`: rewrote malformed `return tN, ok := ...; ok {` as a proper if statement.
- Duplicate `F2` method blocks (IsZero/IsOne/Int declared twice): deleted the second block.
- `F2.Int()`: illegal `uint(self)` bool->int conversion rewritten if/else bool-native.
- `NodeConst.Int()`: same illegal conversion rewritten if/else.
- `F2.Add`: illegal `F2(self.Int() ^ that.Int())` rewritten `F2(self != that)` (XOR).
- `F2.Mul`: illegal `F2(self.Int() & that.Int())` rewritten `F2(self && that)` (AND).
- `F2.Sub`: unchanged (01-02 owns that math); compiles once Add was fixed.
- `type F2 = bool` → `type F2 bool`: alias cannot carry methods ("cannot define new
  methods on non-local type F2"); defined type is the minimal repair that satisfies the
  plan-mandated F2 method block. Add/Mul/IsZero/IsOne bodies wrapped in explicit
  `F2(...)`/`bool(...)` conversions where the defined type requires them.
- `NodeBinary.IsEqual`: `that.l`/`that.r` (fields absent on Expr interface) → `tN.l`/`tN.r`.
- `IsStaticExprRecursive`: undefined `root` → parameter `e`; unused type-switch guard
  variable; missing `return nil` after the switch so the closure satisfies `func(Expr) error`.
- `IsExprZero`/`IsExprOne`: `(*NodeConst).IsZero()/IsOne()` → `Value().IsZero()/IsOne()`.
- `doSymbolize`: arrow-func syntax `func(e Expr) -> bool` → `func(e Expr) bool`;
  trailing comma in `return nil,` removed.
- Added `"fmt"` import (fmt.Errorf used by IsStaticExprRecursive).

### internal/f2/visit.go
- `VisitBy`: bare `int order` parameter named (`order int`); signature normalized to the
  plan-specified single-line form so the acceptance grep matches.
- `Visit`: undefined `expr` → `root`; stray `)` closing the function body → `}`.
- `VisitPost`: same two repairs with `OrderPost`.

### internal/f2/fold.go
- `rewriteEngine` map value type `rewriteRule` → `rewriteRules` (the interface name).
- Type declaration `rwriteFold` → `rewriteFold` (typo'd name broke init()).
- `rewriteFold` case #1 (full-constant fold): illegal `NewConst(F2(lval ^ rval))` /
  `NewConst(F2(lval & rval))` uint->bool conversions rewritten bool-native
  (`lval != rval` for Add, `lval & rval != 0` for Mul), preserving the rule's math.
- `rewriteCommute` case #2: `bin.Op` field access → `bin.Op()`; NewBinary tree nested
  with the operator (`NewBinary(op, NewBinary(op, l1, r), l2)`).
- `rewriteCommute` case #1: undefined `IsStatic` → `IsStaticExpr`.
- `rewriteIdempotent`: `bin.Op` → `bin.Op()`; added missing `return nil, nil`.
- `rewriteReassociation` case 1: `binR.L()`/`binR.R()` on Expr → `rNode.L()`/`rNode.R()`;
  added missing `, nil` error value.
- `rewriteDistribution`: `NewBInary` typo → `NewBinary`; final return gained `, nil`.
- `rewriteNormalization`: `IsStaticExprRecursiv` typo → `IsStaticExprRecursive`;
  final return gained `, nil`.
- `rewriteSymbol` case #2: `l1Node.Op()` → `lNode.Op()`; NewBinary tree nested with
  operator as outer/inner args.
- `folder.run`: gained `Expr` return type and returns the fixed-point expression so
  `Fold(Expr) Expr` compiles.
- Driver loop: unused map key variable dropped.

### internal/f2/coeff_mat.go
- `NewCoeffMatAllZero`: flat `[]Expr` → `[][]Expr` rows of `NewConstZero()`.
- `ToCoeffMat` and `ToConst` loops: `for _, row := self.v` → `for _, row := range self.v`.
- `ToConst`: unused loop index dropped; `ConstMat{vv: ...}` → `ConstMat{v: ...}`.
- `CoeffMat.IsConst`: `ele.IsSymbol()` (not on Expr interface) → `ele.Kind() == Symbol`.
- `CoeffMat.Apply`: missing closing paren on the append; `append(cur, v)` argument swap
  → `append(v, cur)`; `NewVectorAndFold` → `NewVectorWithFold`.
- Added `"fmt"` import.

### internal/f2/vector.go
- `CeoffMatAll() (CeoffMat, error)` → `CoeffMatAll() (CoeffMat, error)`.
- `doCoeffMat`: `tLinearCoeff, err = ...` → `:=`; error branch returns `CoeffMat{}, err`;
  `NewCoeffMatFromVectorList` → `NewCoeffMat`; `check.SlowCheck` → `check.Check(closure())`.
- `extractLinearCoeffVector` marker pass: added missing `return nil`; type-switch guard
  variable now used (`node.L()/node.R()` in the binary case).
- Linearity check: `VisitPre` → `Visit` (no VisitPre exists; Visit is pre-order).
- Marker==0 affine guard: `expr.(*NodeConst).IsZero()` → `Value().IsZero()`; error
  branch returns `nil, fmt.Errorf(...)`.
- `forwardVisit`: signature `func(Expr) error` → `func(parent *NodeBinary, e Expr) error`;
  recursive calls `forwardVisit(bin, bin.L()/bin.R())`; entry `forwardVisit(nil, expr)`;
  `parernt.Op()` typo → `parent.Op()`; `check.NotNil` → `check.CheckNotNil`; two unused
  marker locals removed; bare `return` → `return nil`.
- `out = make(...)` undeclared → `out := make([]Expr, 0, len(order))`.
- `NewVectorWithFold` return type `Vecotr` → `Vector`.
- `NewFromHardware`: `bounds` references → `bits` parameter; `bit = bits[idx]` → `:=`.
- `NewFromSemanticMapping`: `namePrefix[idx]` → `namePrefix[dim]`; `NewVaraible` →
  `NewVariable`; Sprintf format/arg mismatch fixed to `("%s%d_%d", name, dim, c)`
  preserving the documented `name<dim>_<idx>` pattern (vet-clean).
- Added `GetExprVariableNameSet(e Expr) []string` — sorted distinct variable names.
- Added `Vector.Slice(start, end int) Vector` (copied sub-slice) and
  `Vector.Set(idx int, e Expr)` — required by swizzle_map.go.
- Added `"fmt"`, `"sort"` imports; removed unused `common` import.
- `varExpr.Name()` on Expr → `varExpr.(*NodeVariable).Name()`.

### internal/f2/matop.go
- Added `"fmt"` import.
- `PermutationMatrix`: `r.IsEqual(j)` (int as Expr) → `r.IsEqual(l)`; error returns gain
  `CoeffMat{}` value; final `return out` gains `, nil`.
- `EliminationResult`: missing `struct` keyword added.
- Pivot search: `pivot == Zero` (testing the wrong variable) → `candidatePivot != Zero`;
  undeclared `lhsRaw = raw[j]` line removed (swapRow performs the exchange).
- `++cursorJ` (invalid pre-increment) → `cursorJ++`.
- `RankConstMat`: `GuassianElimination(input)` → `GuassianEliminationConstMat(input)`.
- `ShiftMatrix`: rewritten — undefined `NewCoeffMatrisAllZeros`/`NewConstMatrixAllZeros`
  replaced with `NewCoeffMatAllZero` (k >= size) or `[][]F2` built directly and returned
  via `NewConstMat(raw).ToCoeffMat()` (pointer-receiver call on addressable local).
- Removed unused `result` declaration.

### internal/f2/access_map.go
- `perMatrix` → `permMatrix`; unused `common` import block removed.

### internal/f2/stride_map.go
- Added missing `check` package import.

### internal/f2/swizzle_map.go
- Added `"fmt"` and `"strings"` imports.
- `swizzleinfo{}` → `swizzleVectorInfo{}`.
- Free-index loop: `newVector` → `input`; `freeIdx = append(idx, i)` → `append(freeIdx, i)`;
  `freeVector := input.Gather(freeIdx)`.
- vectorInfo error handling restructured: `if err != nil { return Vector{}, err }` first,
  then `if result.Rank == 0 { return Vector{}, SwizzleErrorNoFreeDegree }` (the old
  `return -1, ...` form cannot compile — return type is Vector), then basis-index mapping
  via `freeIdx[bIdx]` unchanged.
- Final swizzle loop: `bankSkip` → `bank.Skip`; `freeVector[...]` struct indexing →
  `freeVector.Index(...)`; `NewBinary(Xor, existedValue)` one-operand call → two-operand
  XOR combining existing value with the free basis expression, stored in `swizzleValue`
  and passed to `input.Set(inputIdx, swizzleValue)`; loop guarded by
  `readyToBeSwizzledDimensionSize` so `freeBasisIdx[idx]` cannot go out of range.
- Added `NameBelong(name, prefix string) bool` (strings.HasPrefix).
- Added `BelongVariableNameFamily` — implemented as a package-level function
  `BelongVariableNameFamily(e Expr, prefix string) bool` instead of the plan-specified
  method on `Expr`, because Go forbids methods on interface receiver types; the single
  call site was updated accordingly.

### internal/check/check.go
- No content changes. The package was untracked in git; committed (bff9978) so a clean
  checkout builds — `internal/f2` imports it throughout.

## Deviations from Plan

### Auto-fixed Issues (blocking compile errors not enumerated in the plan)

**1. [Rule 3 - Blocking] `type F2 = bool` alias cannot carry methods**
- **Found during:** Task 2
- **Issue:** With the alias, every `func (self F2) ...` declaration fails with
  "cannot define new methods on non-local type F2". The plan mandates the F2 method
  block (acceptance greps count `func \(self F2\) ...` matches), which is impossible
  on an alias.
- **Fix:** Changed to defined type `type F2 bool`; wrapped bool expressions in
  `F2(...)`/`bool(...)` conversions where the defined type requires them
  (Add/Mul/IsZero/IsOne bodies). Constants `Zero`/`One` still assign fine.
- **Files modified:** internal/f2/expr.go
- **Commit:** b31e9f4

**2. [Rule 3 - Blocking] Method on Expr interface receiver (`BelongVariableNameFamily`)**
- **Found during:** Task 3
- **Issue:** The plan specified `func (e Expr) BelongVariableNameFamily(prefix string) bool`
  and stated interface-receiver methods are legal; Go rejects them
  ("invalid receiver type Expr (pointer or interface type)").
- **Fix:** Implemented as `func BelongVariableNameFamily(e Expr, prefix string) bool`
  with the call site updated. Behavior identical to the plan's intent.
- **Files modified:** internal/f2/swizzle_map.go
- **Commit:** 5d50e39

**3. [Rule 1 - Bug] Latent compile errors in code the plan did not enumerate**
- **Found during:** Tasks 2-3 (each surfaced by the compiler after the previous layer
  was fixed; the plan's "~10 syntax errors and dozens of latent compile errors" wording
  anticipated this class)
- **Issues and fixes:**
  - fold.go: `rwriteFold` type-name typo → `rewriteFold`; `IsStatic` → `IsStaticExpr`;
    `binR.L()/R()` on Expr → `rNode` accessors; missing `, nil` in reassociation case 1;
    unused map-key variable in driver loop.
  - coeff_mat.go: unused loop index in ToConst; `ConstMat{vv:}` → `{v:}`.
  - matop.go: `++cursorJ` invalid pre-increment → `cursorJ++`; unused `result`
    declaration; pointer-receiver `ToCoeffMat` on non-addressable literal → local var.
  - vector.go: unused `common` import; `e.L()/e.R()` in marker pass → type-switch node;
    `VisitPre` → `Visit`; `varExpr.Name()` on Expr → `(*NodeVariable)` accessor.
  - expr.go: `NodeBinary.IsEqual` used `that.l/that.r` (fields on Expr) → `tN.l/tN.r`.
- **Files modified:** internal/f2/fold.go, coeff_mat.go, matop.go, vector.go, expr.go
- **Commits:** b31e9f4, 5d50e39

### Notes on Acceptance Criteria

- Task 2 criterion 4 (typo grep must output 0) is self-contradictory: the pattern
  `IsStaticExprRecursiv` is a prefix of the correctly-named `IsStaticExprRecursive`,
  which must exist (it is in the plan's interfaces contract). The four grep matches in
  expr.go/fold.go/vector.go are all legitimate uses of the correct name; no actual typo
  remains.
- Task 3 criterion for `BelongVariableNameFamily` method-form signature is unachievable
  (see Deviation 2); the function-form signature was verified instead.

## Verification (final gate, run from repo root)

```
$ go build ./...   # exit 0
$ go vet ./...     # exit 0
$ git diff --stat -- go.mod   # empty (byte-identical; go.sum absent, zero deps)
$ grep -rn "-> bool" internal/f2/                          # no matches
$ grep -rnE "uint\(self|int\(self\)|F2\((lval|rval|self\.Int)" internal/f2/  # no matches
$ grep -rn "perMatrix\|swizzleinfo\|lhsRaw\|NewVectorAndFold\|NewBInary\|CeoffMat\|Vecotr\|NewVaraible\|parernt\|newVector" internal/f2/  # no matches
```

## Auth Gates

None — pure local library work.

## Known Stubs

None. No hardcoded empty values, placeholder text, or unwired data sources were
introduced; all repairs are mechanical compile fixes of user-authored code. Semantic
correctness of fold math, F2 arithmetic edge cases, and matrix/vector operator order is
explicitly deferred to plan 01-02 per the plan boundary.

## Self-Check: PASSED

- All 9 key files exist under internal/f2/ (verified on disk).
- internal/check/check.go exists and is tracked.
- Commits e24bc5b, b31e9f4, 5d50e39, bff9978 present in git log.
- go build ./... and go vet ./... both exit 0; go.mod unchanged.
