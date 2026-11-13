---
phase: quick-261113-gll
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/f2/vector.go
  - internal/f2/matop.go
  - internal/f2/coeff_mat.go
  - internal/f2/swizzle_map.go
  - internal/f2/stride_map.go
  - internal/f2/fold.go
  - internal/check/check.go
  - internal/f2/warning_fixes_test.go
  - internal/f2/pipeline_test.go
  - internal/check/check_test.go
autonomous: true
requirements: [ALG-03, ALG-04, MAP-01, MAP-02, MAP-03, TEST-01, TEST-02]

must_haves:
  truths:
    - "All 15 warnings WR-01..WR-15 from 01-REVIEW.md are fixed; each fix has a regression test that fails on the pre-fix code"
    - "go build ./... and go vet ./... pass; full test suite is green"
    - "ShiftL/ShiftR vacated slots hold ConstZero (no nil Expr holes anywhere downstream)"
    - "SwizzleMap returns SwizzleErrorNoFreeDegree when no free dimensions exist instead of panicking, and validates bank range against input size"
    - "Fold surfaces a check.Bug panic when the iteration cap is exhausted before canonical form"
    - "A full layout pipeline (hardware bits -> MapAccess -> StrideMap -> CoeffMatAll -> elimination/rank -> SwizzleMap) produces a final vector whose per-bit expressions are exact, Fold-idempotent trees directly interpretable into index arithmetic"
    - "MapAccess(hw=[w_0,l_0,l_1], [l_0,l_1,w_0]) returns [l_1,w_0,l_0] per current code behavior, and the PermutationMatrix doc-comment vs behavior mismatch is recorded as a finding in SUMMARY.md"
    - "The residual x + S*x silent-drop shape in extractLinearCoeffVector (coefficient returns M instead of M+1 for bare-variable-plus-product inputs) is documented as a known-limitation finding in SUMMARY.md, not fixed in this plan"
  artifacts:
    - path: "internal/f2/warning_fixes_test.go"
      provides: "Regression tests for WR-01..WR-15 (one focused test per fix)"
      min_lines: 200
    - path: "internal/f2/pipeline_test.go"
      provides: "End-to-end layout-pipeline tests asserting exact output trees"
      min_lines: 150
    - path: "internal/check/check_test.go"
      provides: "Tests for check.fail non-string-message fallback (WR-14)"
      min_lines: 30
  key_links:
    - from: "internal/f2/vector.go doCoeffMat"
      to: "Fold"
      via: "fold each SymbolizeExcept output before extraction (WR-01)"
      pattern: "Fold\\(SymbolizeExcept"
    - from: "internal/f2/swizzle_map.go Run"
      to: "input.Slice"
      via: "Skip+Range bounds validation before slicing (WR-07)"
      pattern: "Skip \\+ bank\\.Range|bank\\.Skip \\+ bank\\.Range"
---

<objective>
Fix all 15 review warnings (WR-01..WR-15) from `.planning/phases/01-build-fix-working-algebra-core/01-REVIEW.md` with minimal surgical repairs and one regression test per fix, then add comprehensive layout-pipeline test coverage exercising the full chain: hardware coordinate bits → expression vector → Fold → CoeffMatAll → Gaussian elimination / rank → AccessMap / StrideMap / SwizzleMap → final codegen-ready vector.

Purpose: The matrix/elimination/swizzle layer is now correct after CR-01..CR-07, but 15 latent defects (silent wrong answers, nil holes, panics on ordinary inputs) remain between it and trustworthy codegen. The pipeline tests lock the library's core value: a final vector whose per-bit expressions are exact, canonical trees a user can interpret into index arithmetic.
Output: 7 patched source files (surgical, no restructuring), 3 new test files.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/STATE.md
@.planning/phases/01-build-fix-working-algebra-core/01-REVIEW.md
@internal/f2/vector.go
@internal/f2/matop.go
@internal/f2/coeff_mat.go
@internal/f2/swizzle_map.go
@internal/f2/stride_map.go
@internal/f2/fold.go
@internal/check/check.go
@internal/f2/review_fixes_test.go

<interfaces>
Established conventions the executor must match (verified in source):
- Tests are `package f2` (white-box), stdlib `testing` only, table-driven where natural. Reuse `mustConst(t, e, want)` and the `constCoeffMat(t, [][]F2)` helpers already defined in `internal/f2/review_fixes_test.go`.
- Panic-assert idiom: `defer func() { r := recover(); if r == nil { t.Fatal("expected panic") } ... }()` — assert the recovered message contains the expected substring (e.g. `"CHECK"`, `"BUG"`).
- Pointer receivers + in-place mutation are the established style — do NOT immutabilize.
- `check.Check` / `check.Bug` panic with messages of the form `KIND failed: <msg> (<file>:<line>)`.
- Constructors: `NewVariable(name)`, `NewSymbol(name)`, `NewConst(F2)`, `NewConstZero()`, `NewConstOne()`, `NewBinary(op, l, r)` with `Add`(Xor)/`Mul`(And); `NewVector([]Expr)`, `NewFromHardware(bits, prefixes)`.
- `Expr.IsEqual(other)` is structural equality — exact-tree assertions use it against trees built with the same constructors (wrap the `want` side in `Fold(...)` when the fold engine's canonical ordering is involved).
- Fold canonical form is left-associative with static on the left (per `rewriteCommute`).
- Error vars already exported: `SwizzleErrorUnknownBasis`, `SwizzleErrorSaturated`, `SwizzleErrorNoFreeDegree` (swizzle_map.go).
- PermutationMatrix (matop.go) doc comment says "A * rhs = lhs" but the code marks `out.On(i, j)` where `rhs[i] == lhs[j]`, building A such that A·lhs = rhs — the INVERSE of the documented direction. Behavior is locked by pipeline test A; the doc mismatch is reported as a finding (doc-comment fix authorized, behavior change NOT authorized).
- WR-fix coverage ledger (all 15 fixed; none deferred — none are design-level):
  WR-01→Task 1, WR-02→Task 1, WR-03→Task 1, WR-04→Task 1, WR-08→Task 1, WR-09→Task 1, WR-11→Task 1, WR-15→Task 1, WR-05→Task 2, WR-06→Task 2, WR-07→Task 2, WR-10→Task 2, WR-12→Task 2, WR-13→Task 2, WR-14→Task 2.
- Known limitation NOT fixed in this plan (document in SUMMARY): `extractLinearCoeffVector`'s forwardVisit (vector.go ~line 389) silently overwrites duplicate same-name closures (`exprMap[name] = e`), so input `x + M*x` (bare variable plus same-variable product, which Fold does NOT merge — normalization only fires for S1*x + S2*x with both sides Mul) drops the bare `x` term and returns coefficient M instead of M+1. Merging there is NOT safe: a merged node is a fresh tree with marker 0, failing `check.Check(c.Marker() == 1)` at vector.go ~line 427, and the merged `Add(x, M*x)` shape fails step [3]'s `c.R().Kind() == Variable` check anyway.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Fix core-algebra warnings WR-01..WR-04, WR-08, WR-09, WR-11, WR-15 + regression tests</name>
  <files>internal/f2/vector.go, internal/f2/matop.go, internal/f2/coeff_mat.go, internal/f2/warning_fixes_test.go</files>
  <behavior>
    - WR-01: `CoeffMatAll()` on `[(M*x) + (N*x)]` (symbols M, N; variable x) yields coefficient of x equal to `Fold(M+N)`, not just M or N. (This input folds to a single closure for x, so it exercises the doCoeffMat fix without touching the known-limitation shape.)
    - WR-02: `ShiftR(1, true)` on `[a, b]` → size 3, Index(0) is ConstZero, Index(1)==a, Index(2)==b. `ShiftL(1, true)` on `[a, b]` → size 3, Index(0)==a, Index(1)==b, Index(2) is ConstZero. No nil entries (calling `.Kind()` on every slot must not panic).
    - WR-03: `IsPerm` on [[1,0],[1,0]] → false; on [[0,1],[1,0]] → true.
    - WR-04: `IsIdentity` on non-square 2x3 all-const matrix [[1,0,0],[0,1,0]] → returns false (this diagonal-1 shape returns true pre-fix, exercising the false-assertion path). `IsIdentity` on non-square 3x2 all-const matrix [[1,0],[0,1],[0,0]] → returns false without panicking (pre-fix this shape panics: m > n, so `self.v[i][i]` indexes past the row length).
    - WR-08: `Vector.Add` on size-mismatched vectors panics with a message containing "CHECK"; equal sizes still work.
    - WR-09: `PermutationMatrix` with `rhs.Rank() != lhs.Rank()` returns a non-nil error (no panic). `PermutationMatrix(rhs=[a,a], lhs=[a,b])` returns a non-nil error (previously silently produced a non-permutation).
    - WR-11: `NewFromSemanticMapping(dims, namePrefix)` with `len(namePrefix) < len(dims)` panics with "CHECK" (previously index-out-of-range).
    - WR-15: `ShiftMatrix(3, -1)` panics with "CHECK" (previously negative-index panic).
  </behavior>
  <action>
Apply the review's recommended fixes surgically; do not restructure surrounding code.

WR-01 (vector.go doCoeffMat, ~line 238): fold each symbolized expression before extraction — change `tVec = append(tVec, SymbolizeExcept(expr, basis))` to `tVec = append(tVec, Fold(SymbolizeExcept(expr, basis)))`. This single change satisfies the review's "and/or" — do NOT also merge duplicate same-name closures in `extractLinearCoeffVector`'s forwardVisit (vector.go ~line 389). Probe-confirmed, that merge converts a silent wrong answer into a reachable panic: the merged node is a fresh tree with marker 0, failing `check.Check(c.Marker() == 1)` (vector.go ~line 427), and the merged `Add(x, M*x)` shape fails step [3]'s `c.R().Kind() == Variable` check anyway. Record the residual `x + S*x` silent-drop shape as a documented known-limitation finding in SUMMARY.md (pre-fix AND post-fix it silently drops the bare variable term; coefficient returns M instead of M+1).

WR-02 (vector.go ShiftL ~line 90 and ShiftR ~line 111): after allocating `vec` and before the copy loop, fill every slot: `for i := range vec { vec[i] = NewConstZero() }`. Keep the existing position logic untouched (CR-06 fix is correct and tested).

WR-03 (coeff_mat.go IsPerm ~line 128): track seen columns. For each row, record the column j of its single 1; if a row has != 1 ones, or its column was already claimed by an earlier row, return false. (Column-uniqueness check per review.)

WR-04 (coeff_mat.go IsIdentity ~line 146): add `if self.m != self.n { return false }` as the first statement (after the IsConst check is fine, but before any `self.v[i][i]` indexing).

WR-08 (vector.go Add ~line 55): add `check.Check(self.Size() == that.Size(), "vector add size mismatch: %d vs %d", self.Size(), that.Size())` before the loop.

WR-09 (matop.go PermutationMatrix ~line 14): (a) replace `check.Check(lhs.Rank() == rhs.Rank())` with `if lhs.Rank() != rhs.Rank() { return CoeffMat{}, fmt.Errorf("permutation matrix: rank mismatch %d vs %d", lhs.Rank(), rhs.Rank()) }`. (b) add a `used := make([]bool, lhs.Rank())` slice; inside the match, if `used[j]` is already true return an error (`fmt.Errorf("permutation matrix: duplicate match at column %d", j)`); set `used[j] = true` on match. Do NOT change the `out.On(i, j)` marking direction or the doc comment here — the doc/behavior mismatch is handled as a finding (Task 3).

WR-11 (vector.go NewFromSemanticMapping ~line 534): add `check.Check(len(namePrefix) >= len(dims), "semantic mapping: %d dims but %d prefixes", len(dims), len(namePrefix))` at function top (index is `namePrefix[dim]` for `dim < len(dims)`, so `>=` is the correct guard).

WR-15 (matop.go ShiftMatrix ~line 156): add `check.Check(k >= 0, "shift matrix: negative k %d", k)` at function top.

Create `internal/f2/warning_fixes_test.go` (`package f2`, stdlib only) with one focused test per fix implementing the `<behavior>` list; name tests `TestFixWR01`..`TestFixWR15`-style (e.g. `TestFixWR01CoeffMatFold`, `TestFixWR04IsIdentityNonSquare`, ...). Match the style of review_fixes_test.go: small helpers, table-driven where natural, `mustConst` reuse, panic tests via defer/recover asserting message substrings. For WR-01 build the want-side as `Fold(NewBinary(Add, m, n))` to respect canonical ordering. For WR-04 use the exact matrices from the behavior list (2x3 [[1,0,0],[0,1,0]] for the false-assertion path, 3x2 [[1,0],[0,1],[0,0]] for the pre-fix panic path).
  </action>
  <verify>
    <automated>go build ./... && go vet ./... && go test ./internal/f2/ -run 'TestFixWR' -count=1 && go test ./internal/f2/ -count=1</automated>
  </verify>
  <done>All 8 fixes applied; new test file exists with ≥8 focused tests named to match the -run pattern; `go test ./...` green (pre-fix, the new tests fail or panic on WR-01/02/03/04/08/09/11/15 inputs); existing CR regression tests still pass.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Fix map/fold/check warnings WR-05..WR-07, WR-10, WR-12..WR-14 + regression tests</name>
  <files>internal/f2/swizzle_map.go, internal/f2/stride_map.go, internal/f2/fold.go, internal/check/check.go, internal/f2/warning_fixes_test.go, internal/check/check_test.go</files>
  <behavior>
    - WR-05: `SwizzleMap(Bank{Skip:2, Range:5, VariablePrefix:"l"}, input)` on an 8-bit input whose indices 2..6 hold bank bits `[l_0, l_0, l_1, l_2, l_3]` returns `SwizzleErrorNoFreeDegree` — the free-bit scan must start at index Skip+Range=7 (index 6 is a bank bit and must never be eligible as a free XOR source). Pre-fix code scans from `bank.Range + 1` = 6, finds index 6 (`l_3`), and swizzles using bank bit `l_3` as the free source — this test is the observable distinguisher (freeIdx is function-local and not directly assertable).
    - WR-06: `SwizzleMap` on input `[t0, t0, w0]` with `Bank{Skip:0, Range:2, VariablePrefix:"t"}` (bank rank 1 < 2, no free bits) returns `SwizzleErrorNoFreeDegree`, not a panic.
    - WR-07: `SwizzleMap` with `Skip+Range > input.Size()` returns a non-nil error, not a slice-bounds panic.
    - WR-10: `Fold` on an input that does not reach canonical form within the iteration cap panics with a message containing "BUG" (previously silently returned a non-canonical expression).
    - WR-12: `StrideMap(tileShape, v, 3)` returns a non-nil error; `dtypeSize` 0, 1, 2 all succeed.
    - WR-13: `NameBelong("thr1_0", "thr1")` true; `NameBelong("thr10_0", "thr1")` false; `NameBelong("t0", "t")` true; `NameBelong("t_0", "t")` true; `NameBelong("thr1_0", "thr")` true.
    - WR-14: `check.Check(false, 42)` panics with a message containing "CHECK failed" and NOT containing "%!" (previously panicked inside fail with a type-assertion error).
  </behavior>
  <action>
WR-05 (swizzle_map.go ~line 178): change the free-bit scan start from `bank.Range + 1` to `bank.Skip + bank.Range`. Verify the existing `TestSwizzleMapEndToEnd` still passes (its index 2 is ConstZero, so freeIdx is unchanged).

WR-06 (swizzle_map.go Run, before the `vectorInfo(freeVector)` call ~line 191): add `if len(freeIdx) == 0 { return Vector{}, SwizzleErrorNoFreeDegree }`. This makes the error reachable for the empty-free-vector case instead of panicking inside `NewCoeffMat`'s `check.Check(m > 0)`.

WR-07 (swizzle_map.go Run top, ~line 127 after the existing bank checks): add `if bank.Skip + bank.Range > input.Size() { return Vector{}, fmt.Errorf("swizzle error: bank skip+range (%d+%d) exceeds input size %d", bank.Skip, bank.Range, input.Size()) }` before the `input.Slice(bank.Skip, bank.Skip+bank.Range)` call.

WR-10 (fold.go): (a) make `doRun` propagate the rewrite error: change its signature to `func (self *folder) doRun(expr Expr) ([]Replacement, error)` returning `self.rewrite(expr)`'s error, and update `run` to receive it. (b) In `run` (~line 410): track convergence — after the loop, if the last `doRun` returned a non-empty replacement queue (cap exhausted without fixpoint), call `check.Bug("fold: iteration cap %d exhausted before canonical form", self.maxBound)`. If `doRun` returns an error, call `check.Bug("fold: rewrite error: %v", err)`. Per project constraint: use `check.Bug` (the established panic-based assertion idiom), not a silent return and not a new error-returning public API. Keep `Fold(expr) Expr` signature unchanged.

WR-12 (stride_map.go): at function top (after the tileShape check), add `if dtypeSize < 0 || dtypeSize > 2 { return Vector{}, fmt.Errorf("stride map: dtypeSize must be 0..2, got %d", dtypeSize) }`; add `"fmt"` to imports. Keep the existing tileShape check as-is.

WR-13 (swizzle_map.go NameBelong ~line 40): implement the boundary-aware rule: let `rest = name[len(prefix):]` after confirming `strings.HasPrefix(name, prefix)`. Return true iff `rest == ""`, OR `rest[0] == '_'`, OR (`rest[0]` is an ASCII digit AND the last character of `prefix` is NOT an ASCII digit). This keeps the hardware convention (`t` matching `t0`, `t_0`) and the semantic convention (`thr` matching `thr1_0`, `thr1` matching `thr1_0`) working while rejecting the family collision (`thr1` vs `thr10_0`, where the digit in `prefix` makes the continuation ambiguous). Document the rule in the function comment.

WR-14 (check/check.go fail ~line 15): replace `expr = fmt.Sprintf(a[0].(string), a[1:]...)` with the comma-ok form: `if format, ok := a[0].(string); ok { expr = fmt.Sprintf(format, a[1:]...) } else { expr = fmt.Sprint(a...) }`.

Append WR tests to `internal/f2/warning_fixes_test.go` (same file, same conventions) covering the `<behavior>` list; name them to match the `TestFixWR` -run pattern from Task 1 (e.g. `TestFixWR05FreeScanStart`, `TestFixWR06NoFreeDegree`, ...). WR-05's test uses the exact 8-bit / Skip:2,Range:5 config from the behavior list — post-fix `SwizzleErrorNoFreeDegree`, pre-fix a successful swizzle using bank bit `l_3`. WR-10's test is white-box: in `package f2` you can build `&folder{maxBound: 1}` — first find an input that needs ≥2 rewrite rounds (probe candidates in a scratch run, e.g. nested constant/commute patterns like `((x + 1) + 1)` or a Mul/Add mix; confirm empirically that `maxBound: 1` leaves a non-empty queue), then assert `f.run(expr)` panics with "BUG" in the message; also assert `Fold` on ordinary expressions still converges (existing fold tests cover this). WR-13 is a pure function — table-driven test on NameBelong directly. WR-14 lives in new `internal/check/check_test.go` (`package check`): defer/recover around `Check(false, 42)` asserting "CHECK failed" present and "%!" absent.
  </action>
  <verify>
    <automated>go build ./... && go vet ./... && go test ./internal/f2/ -run 'TestFixWR' -count=1 && go test ./internal/check/ -count=1 && go test ./... -count=1</automated>
  </verify>
  <done>All 7 fixes applied; WR tests appended (f2, named to match the -run pattern) and check_test.go created; `go test ./...` green; pre-fix behavior verified failing for each new test (panic where an error is now returned, silent answer where Bug is now raised).</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Comprehensive layout-pipeline test coverage (codegen-ready final vector)</name>
  <files>internal/f2/pipeline_test.go, internal/f2/matop.go</files>
  <behavior>
    Pipeline test A (`TestPipelineAccessMap`, MAP-01): `hw := NewFromHardware([]int{1, 2}, []string{"w", "l"})` gives `[w_0, l_0, l_1]`. `MapAccess(hw, NewVector([l_0, l_1, w_0]))` returns `[l_1, w_0, l_0]` — probe-confirmed: PermutationMatrix marks `out[i][j]` where `rhs[i] == lhs[j]`, building A such that A·lhs = rhs, the INVERSE of its doc comment "A * rhs = lhs". Assert exactly `[l_1, w_0, l_0]` per current code behavior.
    Pipeline test B (`TestPipelineStrideMap`, MAP-03 + WR-02 lock): `StrideMap([]int{0,0,0}, vecFromA, 1)` gives size-4 vector `[l_1, w_0, l_0, 0]` (vecFromA is `[l_1, w_0, l_0]` and ShiftL preserves element order) with Index(3) a ConstZero (not nil). `StrideMap(..., 0)` leaves size unchanged. `StrideMap(..., 3)` returns error (WR-12 lock).
    Pipeline test C (`TestPipelineSwizzle`, MAP-02): input `[l_0, l_0, l_1, w_0]`, `Bank{Skip:0, Range:2, VariablePrefix:"l"}` (this config panics pre-WR-05/06-fix, so it is only valid as a post-fix assertion): the swizzle lands on slot 1 with tree exactly `NewBinary(Xor, l_0, l_1)` (existed value on the left), matching the existing TestSwizzleMapEndToEnd pattern; slots 0, 2, 3 unchanged. Also: same shape with family-mismatched tail (`[l_0, l_0, w_0]`) returns `SwizzleErrorNoFreeDegree` (WR-06 lock).
    Pipeline test D (`TestPipelineCoeffMat`, ALG-04): `NewVector([l_0, (l_0 + l_1)])` → `CoeffMatAll` yields 2x2 matrix [[1,0],[1,1]] (exact const entries via mustConst); `GuassianEliminationConstMat` → rank 2. Rank-deficient case `NewVector([l_0 + l_1, l_0 + l_1])` → rank 1, `BasisIndex == [0]`. Symbolic-coefficient case `NewVector([(M*l_0) + l_1])` → matrix row `[M, 1]` with Index(0,0) IsEqual symbol M and Index(0,1) ConstOne.
    Pipeline test E (`TestPipelineCodegen`, headline): compose a family-consistent chain that actually swizzles — access-map a 4-bit hardware vector to `[l_0, l_0, l_1, l_2]` (dtypeSize 0), then `SwizzleMap(Bank{Skip:0, Range:2, VariablePrefix:"l"})` → slot 1 becomes exactly `Xor(l_0, l_1)` (checker-verified). Assert the exact expected tree on EVERY bit of the final vector via IsEqual, then assert `Fold(final.Index(i))` IsEqual `final.Index(i)` for all i (canonical form — the property that makes per-bit expressions directly interpretable into index arithmetic). Note: configs whose bank bits contain no variable of the bank's family return `SwizzleErrorUnknownBasis` (e.g. `Bank{..., VariablePrefix:"w"}` on an all-`l` bank) — a swizzle is impossible there, so do not use family-mismatched configs for the headline test. Also assert `MapAccess` on bit-sets that differ returns an error (WR-09 lock through the public API).
  </behavior>
  <action>
Create `internal/f2/pipeline_test.go` (`package f2`, stdlib only, table-driven where natural, reuse `mustConst` and the WR/CR test helpers) with the five tests named exactly `TestPipelineAccessMap`, `TestPipelineStrideMap`, `TestPipelineSwizzle`, `TestPipelineCoeffMat`, `TestPipelineCodegen` (the -run pattern in verify depends on these names). Implement tests A–E from `<behavior>` exactly — every behavior spec above is probe- or checker-verified against current source, so encode them verbatim; do not re-derive or weaken them. Where the fold engine's canonical tree shape for an XOR/addition differs from the naive constructor order, build the want-side with `Fold(NewBinary(...))` and confirm by running the test — the test must assert exact trees, not just variable presence, because exactness IS the codegen-readiness property. Do not modify production source in this task EXCEPT: if the PermutationMatrix doc-comment mismatch finding is confirmed, you MAY correct the doc comment in matop.go (and access_map.go if it repeats the claim) to state A·lhs = rhs; a behavior change to PermutationMatrix is NOT authorized (design decision for the user). If any pipeline test exposes a real bug beyond the documented known limitation, stop and report it in the SUMMARY rather than patching code silently.
  </action>
  <verify>
    <automated>go build ./... && go vet ./... && go test ./internal/f2/ -run 'TestPipeline' -v -count=1 && go test ./... -count=1</automated>
  </verify>
  <done>pipeline_test.go exists with the five named tests covering legs A–E; `go test ./...` green; the headline composed-pipeline test asserts exact per-bit trees plus Fold idempotence on the final vector; any doc-comment fix limited to the A·lhs=rhs wording; findings (doc mismatch, known limitation) recorded in SUMMARY.md.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| (none) | Host-side pure Go algebra library: no IO, no network, no user-facing input parsing, no external processes in this plan. No new dependencies are introduced (all fixes use stdlib + existing internal packages). |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-Q-01 | Tampering | go toolchain / module deps | accept | Zero new third-party imports; `go vet` + full test suite run as the integrity gate; no package-manager install step exists in this plan |
</threat_model>

<verification>
- `go build ./... && go vet ./...` clean
- `go test ./... -count=1` fully green
- Every WR-01..WR-15 id appears in warning_fixes_test.go or check_test.go with a test that fails on the pre-fix code (verified by the executor during TDD red phase)
- No CR-01..CR-07 regression: existing review_fixes_test.go untouched and passing
- Production diffs are surgical: no function renames, no signature changes to exported APIs except PermutationMatrix/StrideMap/SwizzleMap already returning error, no restructuring of user-authored code
- SUMMARY.md records: (1) the PermutationMatrix doc-comment vs behavior mismatch (A·lhs=rhs vs documented A·rhs=lhs) and whether the doc comment was corrected; (2) the residual `x + S*x` silent-drop known limitation in extractLinearCoeffVector
</verification>

<success_criteria>
- All 15 warnings fixed with the review's recommended approach (no deferrals — none are design-level)
- 3 new test files: internal/f2/warning_fixes_test.go, internal/f2/pipeline_test.go, internal/check/check_test.go
- Pipeline tests exercise the full layout-calculation chain on realistic small inputs and assert exact expected trees on the final vector (codegen-readiness property)
- Third-party import count unchanged at zero
</success_criteria>

<output>
Create `.planning/quick/261113-gll-fix-remaining-review-warnings-wr-01-wr-1/261113-gll-SUMMARY.md` when done
</output>
