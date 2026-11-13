---
phase: quick-261113-gll
plan: 01
subsystem: algebra-core
tags: [f2, go, cuda-layout, swizzle, gaussian-elimination, codegen]

# Dependency graph
requires:
  - phase: 01-build-fix-working-algebra-core
    provides: CR-01..CR-07 fixed matrix/elimination/swizzle layer plus review_fixes_test.go helpers (mustConst, constCoeffMat)
provides:
  - All 15 WR-01..WR-15 review warnings fixed, each with a regression test that fails on pre-fix code
  - End-to-end layout-pipeline tests asserting exact Fold-idempotent per-bit trees (codegen-readiness property)
  - check.fail robust against non-string first arg
affects: [codegen, layout-maps, future-review]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "check.Check/Bug call sites pre-format messages with fmt.Sprintf: go vet's printf analysis flags %d/%v in variadic interface{} args"
    - "PermutationMatrix duplicate handling: a claimed lhs column is skipped, not an instant error, so duplicated coordinates map bijectively"

key-files:
  created:
    - internal/f2/warning_fixes_test.go
    - internal/f2/pipeline_test.go
    - internal/check/check_test.go
  modified:
    - internal/f2/vector.go
    - internal/f2/matop.go
    - internal/f2/coeff_mat.go
    - internal/f2/swizzle_map.go
    - internal/f2/stride_map.go
    - internal/f2/fold.go
    - internal/check/check.go

key-decisions:
  - "WR-09 duplicate-match refined from the plan's literal error-on-used-column to skip-and-continue: the literal form makes any access map with a duplicated data coordinate unbuildable, contradicting Task 3's pipeline test E; non-bijective inputs still error"
  - "PermutationMatrix doc comment corrected to A * lhs = rhs; behavior change NOT made (design decision left to the user)"

patterns-established:
  - "TDD RED/GREEN per task: failing regression test committed first, fix committed separately"

requirements-completed: [ALG-03, ALG-04, MAP-01, MAP-02, MAP-03, TEST-01, TEST-02]

# Metrics
duration: 10min
completed: 2026-11-13
---

# Quick 261113-gll: Fix Remaining Review Warnings (WR-01..WR-15) Summary

**All 15 review warnings fixed with per-fix regression tests, plus a 5-test layout-pipeline suite locking exact Fold-idempotent codegen-ready output vectors; PermutationMatrix doc corrected to A·lhs = rhs and its duplicate-match handling refined to permit duplicated coordinates**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-11-13T04:19:50Z
- **Completed:** 2026-11-13T04:29:51Z
- **Tasks:** 3
- **Files modified:** 7 source + 3 test files

## Accomplishments

- WR-01..WR-15 all fixed (silent wrong answers, nil Expr holes, panics on ordinary inputs) with 16 new focused tests that fail on pre-fix code
- Composed pipeline test (MapAccess -> StrideMap -> SwizzleMap) asserting exact per-bit trees and Fold-idempotence on the final vector — the library's core codegen-readiness value
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` fully green; zero third-party imports (unchanged)

## Required Findings (from plan verification criteria)

### 1. PermutationMatrix doc-comment vs behavior mismatch — CONFIRMED and CORRECTED

`PermutationMatrix` (matop.go) marks `out.On(i, j)` where `rhs[i] == lhs[j]`, which builds A such that **A·lhs = rhs** — the inverse of the documented "A * rhs = lhs". The doc comment was corrected (authorized by the plan; Task 3 commit `7de870f`). **No behavior change was made**; pipeline test A locks the current behavior (`MapAccess(hw=[w_0,l_0,l_1], [l_0,l_1,w_0])` returns `[l_1, w_0, l_0]`). If the documented direction was the intended semantics, flipping it is a design decision for the user — it would change MapAccess output ordering.

### 2. Residual `x + S*x` silent-drop known limitation — CONFIRMED, NOT fixed (per plan)

`extractLinearCoeffVector`'s forwardVisit stores one closure per variable name (`exprMap[name] = e`), so a bare-variable-plus-same-variable-product input like `x + M*x` silently drops the bare `x` term and returns coefficient M instead of M+1. Pre-fix AND post-fix behavior is identical here. The WR-01 fix (Fold each symbolized expression) does not merge this shape — Fold's normalization only fires for `S1*x + S2*x` with both sides Mul, and the plan (probe-confirmed) documents that merging in forwardVisit is unsafe: the merged node is a fresh tree with marker 0, failing `check.Check(c.Marker() == 1)`, and the merged `Add(x, M*x)` shape fails step [3]'s `c.R().Kind() == Variable` check anyway. Resolving this needs a design-level change to the coefficient-extraction state machine, out of scope for a warning-fix plan.

## Task Commits

1. **Task 1: Fix WR-01..04, 08, 09, 11, 15 + regression tests** — RED `6c313b5` (test), GREEN `6a5f745` (fix)
2. **Task 2: Fix WR-05..07, 10, 12..14 + regression tests** — RED `91f3fe1` (test), GREEN `99af73f` (fix), vet follow-up `2db634f`
3. **Task 3: Layout-pipeline test coverage** — `7de870f` (feat: pipeline tests + PermutationMatrix doc/dup handling)

## Files Created/Modified

- `internal/f2/warning_fixes_test.go` (created, 327 lines) — 16 tests: TestFixWR01..WR15 covering every fix
- `internal/f2/pipeline_test.go` (created, 280 lines) — TestPipelineAccessMap/StrideMap/Swizzle/CoeffMat/Codegen (A-E)
- `internal/check/check_test.go` (created, 47 lines) — WR-14 non-string-message fallback
- `internal/f2/vector.go` — WR-01 (Fold before extraction), WR-02 (zero-fill shifts), WR-08 (Add size check), WR-11 (prefix length guard)
- `internal/f2/matop.go` — WR-09 (error on rank mismatch; column-claim tracking), WR-15 (negative-k check), doc comment A·lhs=rhs
- `internal/f2/coeff_mat.go` — WR-03 (IsPerm column uniqueness), WR-04 (IsIdentity non-square guard)
- `internal/f2/swizzle_map.go` — WR-05 (scan start Skip+Range), WR-06 (empty free vector error), WR-07 (bank bounds check), WR-13 (family-boundary NameBelong)
- `internal/f2/fold.go` — WR-10 (rewrite-error propagation; check.Bug on cap exhaustion)
- `internal/f2/stride_map.go` — WR-12 (dtypeSize 0..2 validation)
- `internal/check/check.go` — WR-14 (comma-ok fail, fmt.Sprint fallback)

## Decisions Made

- **WR-09 duplicate handling refined** (see Deviations #1): skip a claimed column and continue scanning instead of erroring on first contact. All WR-09 plan behaviors still hold (rank mismatch errors; `rhs=[a,a], lhs=[a,b]` errors).
- **check.Check/Bug messages pre-formatted at call sites** with `fmt.Sprintf`: go vet's printf analysis flags `%d`/`%v` in the variadic `interface{}` args of Check/Bug. Messages are unchanged; this matches the existing codebase idiom of no-format-args Check calls.
- **Task 3 test E composition**: the plan's literal description ("access-map a 4-bit hardware vector to [l_0,l_0,l_1,l_2]") is realized as hw=`[l0,l0,l2,l1]` mapped to data coord `[l0,l0,l1,l2]`. The naive reading (hw=`[l0,l1,l2,l0]`, data=`[l0,l0,l1,l2]`) yields `SwizzleErrorSaturated` because A·hw lands as `[l0,l1,l2,l0]`. The chosen setup produces exactly the plan's specified final vector `[l0, Xor(l0,l1), l1, l2]`.

## TDD Gate Compliance

- Task 1: RED `6c313b5` precedes GREEN `6a5f745` — compliant.
- Task 2: RED `91f3fe1` precedes GREEN `99af73f` — compliant. (RED run confirmed each test failing/panicking on pre-fix code: WR-05 silent success, WR-06 NewCoeffMat panic, WR-07 slice-bounds panic, WR-10 silent return, WR-12 nil error, WR-13 wrong match, WR-14 TypeAssertionError.)
- Task 3: pipeline tests lock probe-verified behavior; the single test-design iteration (test E's hw/data-coord arrangement) is documented above. No plan behavior spec was weakened — the swizzle target tree `Xor(l_0, l_1)` and all other specs encoded verbatim.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug / plan-internal contradiction] WR-09 duplicate-match refined from error-on-used to skip-and-continue**
- **Found during:** Task 3 (TestPipelineCodegen first run)
- **Issue:** Task 2's literal WR-09 fix (`if used[j] { return error }` on first equal-column contact) makes `PermutationMatrix` unable to map ANY lhs containing a duplicated expression — the second match always hits the first claimed column first. That contradicts Task 3's behavior spec, which requires `MapAccess` to produce bank bits `[l_0, l_0]` (duplicated) for the composed pipeline to swizzle. Under the literal fix, test E's `MapAccess` failed with "duplicate match at column 0" and no hardware/data-coord arrangement can satisfy both specs.
- **Fix:** On a claimed column, `continue` scanning for another equal lhs column; error only when no unused equal column exists ("incompatible input vectors"). Rank-mismatch error unchanged. All plan-specified WR-09 behaviors still verified by `TestFixWR09PermutationMatrixErrors` (rank mismatch → error; `rhs=[a,a], lhs=[a,b]` → error).
- **Files modified:** internal/f2/matop.go
- **Verification:** `go test ./internal/f2/ -run 'TestFixWR09|TestPipeline' -count=1` green; full suite green
- **Committed in:** `7de870f` (Task 3 commit)

**2. [Rule 3 - Blocking] check.Check/Bug format directives pre-formatted with fmt.Sprintf**
- **Found during:** Task 2 GREEN verification (`go vet ./...`)
- **Issue:** `go vet` printf analysis flags `%d`/`%v` directives passed in the variadic `interface{}` args of `check.Check`/`check.Bug` (the functions are treated as printf-like wrappers; the codebase had no prior directive-bearing calls, so this was latent). The plan's literal message strings (e.g. `"vector add size mismatch: %d vs %d"`) fail vet.
- **Fix:** Build messages with `fmt.Sprintf` at the call site and pass a single string arg. Message content unchanged. This matches the established codebase idiom (`check.Check(cond)` / plain-string messages elsewhere).
- **Files modified:** internal/f2/vector.go, internal/f2/matop.go, internal/f2/fold.go, internal/check/check_test.go
- **Verification:** `go vet ./...` clean; panic-message tests still assert "CHECK"/"BUG" substrings
- **Committed in:** `99af73f` and `2db634f`

---

**Total deviations:** 2 (1 plan-contradiction refinement, 1 vet compliance)
**Impact on plan:** Both preserve every plan-specified behavior; no scope creep. Deviation #1 is required to satisfy Tasks 2 and 3 simultaneously.

## Issues Encountered

- WR-04 pre-fix panic aborts the whole test binary during the RED run (t.Fatal-defer cannot intercept a hard index-out-of-range in the same goroutine path) — expected; post-fix the panic path returns false. RED status was confirmed per-test with individual `-run` invocations.
- Same abort behavior for WR-06/WR-07 pre-fix panics — confirmed individually.

## Known Stubs

None. Every new test asserts exact behavior against production code; no placeholder values, TODO markers, or unwired data sources were introduced.

## Threat Flags

None. Host-side pure Go algebra library; no IO, network, or new dependencies. Threat model T-Q-01 gate (`go vet` + full test suite) passes.

## Self-Check

- internal/f2/warning_fixes_test.go — FOUND (327 lines, ≥200 required)
- internal/f2/pipeline_test.go — FOUND (280 lines, ≥150 required)
- internal/check/check_test.go — FOUND (47 lines, ≥30 required)
- Commits 6c313b5, 6a5f745, 91f3fe1, 99af73f, 2db634f, 7de870f — FOUND in git log
- `go build ./... && go vet ./... && go test ./... -count=1` — all green (final run at 04:29Z)

## Self-Check: PASSED

## Next Phase Readiness

- The matrix/elimination/swizzle layer is now warning-free and pipeline-tested; the library's core value (exact, canonical, interpretable per-bit layout vectors) is locked by TestPipelineCodegen.
- Open design questions for the user: (1) PermutationMatrix intended direction (A·lhs=rhs implemented vs A·rhs=lhs documented); (2) the `x + S*x` coefficient-extraction limitation, which needs a forwardVisit state-machine redesign to fix safely.
- No blockers.

---
*Phase: quick-261113-gll*
*Completed: 2026-11-13*
