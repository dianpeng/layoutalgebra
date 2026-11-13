---
phase: quick-261113-gll
verified: 2026-11-13T00:00:00Z
status: passed
score: 8/8 must-haves verified
overrides_applied: 2
overrides:
  - must_have: "WR-09: PermutationMatrix with rhs.Rank() != lhs.Rank() returns a non-nil error (no panic). PermutationMatrix(rhs=[a,a], lhs=[a,b]) returns a non-nil error"
    reason: "Plan's literal error-on-used-column refined to skip-and-continue: literal form makes any access map with duplicated data coordinates unbuildable, contradicting Task 3 pipeline test E (duplicated l_0 must map bijectively). All plan-specified WR-09 behaviors still hold (rank mismatch errors; rhs=[a,a], lhs=[a,b] errors) and are locked by TestFixWR09PermutationMatrixErrors. Recorded as deviation #1 in SUMMARY.md."
    accepted_by: verifier (goal-backward verification)
    accepted_at: "2026-11-13T00:00:00Z"
  - must_have: "go build ./... and go vet ./... pass; full test suite is green (with plan-literal Check message strings)"
    reason: "go vet printf analysis flags %d/%v in variadic interface{} args of check.Check/check.Bug. Messages pre-formatted with fmt.Sprintf at call sites; content unchanged. Recorded as deviation #2 in SUMMARY.md."
    accepted_by: verifier (goal-backward verification)
    accepted_at: "2026-11-13T00:00:00Z"
---

# Quick 261113-gll: Fix WR-01..WR-15 + Layout-Pipeline Test Coverage — Verification Report

**Task Goal:** Fix all 15 review warnings WR-01..WR-15 in internal/f2 and add comprehensive F2 algebra test coverage for layout-calculation pipelines (coordinate bits → expression vector → fold → coefficient matrix → elimination/rank → access/swizzle/stride maps → codegen-ready result vector).
**Verified:** 2026-11-13
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | All 15 warnings WR-01..WR-15 fixed; each fix has a regression test that fails on pre-fix code | ✓ VERIFIED | Every fix confirmed in source at the cited location; each test's assertion targets the exact pre-fix failure mode documented in 01-REVIEW.md (see per-WR table below) |
| 2 | go build ./... and go vet ./... pass; full test suite is green | ✓ VERIFIED (override: message pre-formatting) | Ran `go build ./... && go vet ./... && go test ./... -count=1` — all green in this verification session |
| 3 | ShiftL/ShiftR vacated slots hold ConstZero (no nil Expr holes) | ✓ VERIFIED | vector.go:101-104 and 126-129 zero-fill before copy; TestFixWR02ShiftZeroFill calls `.Kind()` on every slot |
| 4 | SwizzleMap returns SwizzleErrorNoFreeDegree when no free dimensions exist; validates bank range against input size | ✓ VERIFIED | swizzle_map.go:218-220 (len(freeIdx)==0 → error) and 150-157 (Skip+Range > Size → error); TestFixWR05/06/07 pass |
| 5 | Fold surfaces check.Bug panic when iteration cap exhausted | ✓ VERIFIED | fold.go:410 doRun returns error; fold.go:424 and 435 call check.Bug; TestFixWR10FoldCapExhausted asserts "BUG" panic with maxBound:1 |
| 6 | Full pipeline (hw bits → MapAccess → StrideMap → CoeffMatAll → elimination/rank → SwizzleMap) yields exact, Fold-idempotent per-bit trees | ✓ VERIFIED | TestPipelineCodegen composes all stages, asserts exact trees via IsEqual on every bit, and asserts Fold(x) IsEqual x for all bits — passes |
| 7 | MapAccess(hw=[w_0,l_0,l_1], [l_0,l_1,w_0]) returns [l_1,w_0,l_0]; PermutationMatrix doc/behavior mismatch recorded as finding | ✓ VERIFIED | TestPipelineAccessMap passes with exact [l_1,w_0,l_0]; matop.go:9 doc comment corrected to "A * lhs = rhs"; SUMMARY.md finding #1 present |
| 8 | Residual x + S*x silent-drop limitation documented in SUMMARY, not fixed in this plan | ✓ VERIFIED | vector.go:403 still `exprMap[name] = e` (single closure per name, overwrite); SUMMARY.md finding #2 documents coefficient M vs M+1 and why merging is unsafe |

**Score:** 8/8 truths verified (2 with accepted overrides)

### Per-WR source spot-checks (all at 01-REVIEW.md cited locations)

| WR | Review's pre-fix defect | Fix found in source | Regression test | Pre-fix failure mode matches review |
|----|------------------------|---------------------|-----------------|-------------------------------------|
| 01 | doCoeffMat never folds symbolized expr (vector.go:231-254) | vector.go:252 `Fold(SymbolizeExcept(expr, basis))` | TestFixWR01CoeffMatFold | Pre-fix coefficient of x = M only; test wants Fold(M+N) |
| 02 | Shift vacated slots nil (vector.go:90-126) | vector.go:102-104, 127-129 zero-fill | TestFixWR02ShiftZeroFill | Pre-fix nil holes nil-panic on `.Kind()`; test probes every slot |
| 03 | IsPerm no column uniqueness (coeff_mat.go:127-143) | coeff_mat.go:131-148 seenCol tracking | TestFixWR03IsPermColumnUniqueness | Pre-fix [[1,0],[1,0]] → true; test asserts false |
| 04 | IsIdentity panics non-square (coeff_mat.go:145-165) | coeff_mat.go:158 `m != n → false` | TestFixWR04IsIdentityNonSquare | Pre-fix 2x3 → true, 3x2 → panic; test asserts false for both |
| 05 | Free scan starts Range+1 (swizzle_map.go:178) | swizzle_map.go:207 `bank.Skip + bank.Range` | TestFixWR05FreeScanStart | Pre-fix swizzles using bank bit l_3; test expects SwizzleErrorNoFreeDegree |
| 06 | Empty free vector panics (swizzle_map.go:188-198) | swizzle_map.go:218-220 early error | TestFixWR06NoFreeDegree | Pre-fix panics in NewCoeffMat m>0; test expects error |
| 07 | Unchecked Slice bounds (swizzle_map.go:131) | swizzle_map.go:150-157 bounds validation | TestFixWR07BankBounds | Pre-fix slice-bounds panic; test expects non-nil error |
| 08 | Vector.Add no size check (vector.go:55-70) | vector.go:56-59 check.Check size match | TestFixWR08VectorAddSizeMismatch | Pre-fix silent truncation/panic; test expects "CHECK" panic |
| 09 | Rank-mismatch panic; non-bijective missed (matop.go:11-33) | matop.go:18-24 error return; 26-47 used-column tracking | TestFixWR09PermutationMatrixErrors | Pre-fix panics / silent non-permutation; test expects errors for both |
| 10 | Fold discards errors, silent cap give-up (fold.go:404-419) | fold.go:410 error-returning doRun; 424/435 check.Bug | TestFixWR10FoldCapExhausted | Pre-fix silent non-canonical return; test expects "BUG" panic |
| 11 | namePrefix length unchecked (vector.go:521-539) | vector.go:549-552 check.Check guard | TestFixWR11SemanticMappingPrefixGuard | Pre-fix index-out-of-range; test expects "CHECK" panic |
| 12 | dtypeSize unvalidated (stride_map.go:18-25) | stride_map.go:27-29 0..2 validation | TestFixWR12StrideMapDtypeSize | Pre-fix nil error for 3; test expects error |
| 13 | NameBelong fragile across families (swizzle_map.go:40-42) | swizzle_map.go:44-60 boundary-aware rule | TestFixWR13NameBelong | Pre-fix thr1 matches thr10_0; table asserts false |
| 14 | fail asserts a[0].(string) unchecked (check.go:15-19) | check.go:20-24 comma-ok + fmt.Sprint fallback | TestCheckNonStringMessage (check pkg) | Pre-fix panics with type-assertion error (no "CHECK failed"); test asserts "CHECK failed", no "%!" |
| 15 | ShiftMatrix panics negative k (matop.go:158-173) | matop.go:174 `check.Check(k >= 0, ...)` | TestFixWR15ShiftMatrixNegativeK | Pre-fix negative-index panic; test expects "CHECK" panic |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/f2/warning_fixes_test.go` | Regression tests WR-01..WR-15, ≥200 lines | ✓ VERIFIED | 327 lines; 14 tests covering WR-01..13+15; all pass under `-run TestFixWR` |
| `internal/f2/pipeline_test.go` | End-to-end pipeline tests, ≥150 lines | ✓ VERIFIED | 280 lines; 5 tests (AccessMap/StrideMap/Swizzle/CoeffMat/Codegen); all pass under `-run TestPipeline` |
| `internal/check/check_test.go` | WR-14 non-string fallback tests, ≥30 lines | ✓ VERIFIED | 47 lines; 2 tests pass |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| vector.go doCoeffMat | Fold | fold each SymbolizeExcept output before extraction (WR-01) | ✓ WIRED | vector.go:252 `Fold(SymbolizeExcept(expr, basis))` matches pattern `Fold\(SymbolizeExcept` |
| swizzle_map.go Run | input.Slice | Skip+Range bounds validation before slicing (WR-07) | ✓ WIRED | swizzle_map.go:150-157 validates before `input.Slice` at line 160 |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full build + vet + tests | `go build ./... && go vet ./... && go test ./... -count=1` | all green (internal/check ok, internal/f2 ok) | ✓ PASS |
| WR regression tests | `go test ./internal/f2/ -run 'TestFixWR' -v -count=1` | 14/14 PASS | ✓ PASS |
| Pipeline tests | `go test ./internal/f2/ -run 'TestPipeline' -v -count=1` | 5/5 PASS | ✓ PASS |
| check package tests | `go test ./internal/check/ -v -count=1` | 2/2 PASS | ✓ PASS |
| Zero third-party imports | grep for external import paths | only `github.com/dianpeng/f2algebra/internal/{check,common}`; no go.sum; go.mod has no requires | ✓ PASS |
| TDD commit order | `git log --oneline` | 6c313b5 (RED) < 6a5f745 (GREEN); 91f3fe1 (RED) < 99af73f (GREEN); all 6 commits present | ✓ PASS |

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|-------------|-------------|--------|----------|
| ALG-03 | F2 vector representation, one expression per output bit | ✓ SATISFIED | TestPipelineAccessMap/StrideMap exercise hw-bit vectors → expression vectors |
| ALG-04 | Coefficient matrices + ops over F2 expressions | ✓ SATISFIED | TestPipelineCoeffMat (exact const entries, rank 2, rank-1 deficient, symbolic row [M,1]) |
| MAP-01 | Access map hw bits → tile offset | ✓ SATISFIED | TestPipelineAccessMap + TestPipelineCodegen stage 1 |
| MAP-02 | Swizzle map construction/composition | ✓ SATISFIED | TestPipelineSwizzle + TestPipelineCodegen swizzle stage |
| MAP-03 | Stride map construction/composition | ✓ SATISFIED | TestPipelineStrideMap + TestPipelineCodegen stride stage |
| TEST-01 | Table-driven stdlib tests for internal/f2 | ✓ SATISFIED | warning_fixes_test.go (327 lines, table-driven WR-13, stdlib only) |
| TEST-02 | Table-driven stdlib tests for layout maps | ✓ SATISFIED | pipeline_test.go (280 lines, all three map types) |

Note: REQUIREMENTS.md checkboxes for these IDs remain unchecked (`- [ ]`) — the features were delivered in phase 01 and this task added the coverage; checkbox updating is bookkeeping outside this task's scope. Informational only.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER in any modified file | — | grep exit 1 (no matches) |

### Findings and Deviations Accuracy (task-specific check)

| Claimed item | Status | Evidence |
|--------------|--------|----------|
| Finding 1: PermutationMatrix doc mismatch, corrected | ✓ ACCURATE | matop.go:9-11 now documents "A * lhs = rhs" with the column-claim explanation; behavior unchanged and locked by TestPipelineAccessMap |
| Finding 2: x + S*x silent-drop limitation, documented not fixed | ✓ ACCURATE | vector.go:403 `exprMap[name] = e` overwrite intact; SUMMARY finding #2 matches source reality |
| Deviation 1: WR-09 skip-and-continue | ✓ ACCURATE & CONTAINED | matop.go:33-37 skips claimed columns; still errors when no unused equal column exists (TestFixWR09 rhs=[a,a],lhs=[a,b] passes); required for TestPipelineCodegen's duplicated l_0 to map (verified by hand-tracing MapAccess arg order in access_map.go) |
| Deviation 2: check message pre-formatting | ✓ ACCURATE & CONTAINED | fmt.Sprintf at call sites (vector.go:58, fold.go:424,435); go vet clean; panic-message tests assert "CHECK"/"BUG" substrings and pass |

### Human Verification Required

None. Pure host-side Go algebra library; no UI, network, or external services. All behaviors verified programmatically by the green suite. Two open design questions (PermutationMatrix direction, coefficient-extraction state-machine redesign) are recorded as findings in SUMMARY.md for the user, but they are explicit non-goals of this task, not verification gaps.

### Gaps Summary

No gaps. All 15 warnings fixed in source with regression tests whose assertions target the exact pre-fix failure modes from 01-REVIEW.md; the composed pipeline test locks exact Fold-idempotent codegen-ready output; project constraints hold (stdlib-only tests, zero third-party imports, surgical diffs, no restructuring — review_fixes_test.go untouched).

---

_Verified: 2026-11-13_
_Verifier: Claude (gsd-verifier)_
