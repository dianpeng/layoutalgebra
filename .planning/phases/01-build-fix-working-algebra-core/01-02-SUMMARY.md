---
phase: 01-build-fix-working-algebra-core
plan: 02
subsystem: f2-algebra-core
tags: [f2-algebra, go, tdd, fold, matrix-vector]
requires:
  - phase: 01-01
    provides: "Compilable internal/f2 package (expression tree, fold engine, coeff matrix, vector) — the baseline these semantic fixes and tests build on"
provides:
  - "Correct F2 scalar semantics: Sub = Add (a+b), proven over the full {0,1}² truth table"
  - "Deterministic fold engine: fixed-order rule list replaces map iteration; six F2 identities + constant-subtree folding proven table-driven"
  - "Correct rewriteNormalization math: S1*x + S2*x => (S1+S2)*x combines coefficients, not coefficient+variable"
  - "Correct CoeffMat.Apply: F2 matrix-vector product y_i = XOR_j (A_ij AND x_j), proven via identity/swap/zero matrices"
  - "Correct Vector.Gather: result order follows the index list"
  - "4 stdlib-only table-driven test files (expr/fold/coeff_mat/vector) locking ALG-01, ALG-02, ALG-05 and ROADMAP criterion 4"
affects:
  - "Any plan consuming Fold canonical forms (determinism now guaranteed)"
  - "matop.go Gaussian elimination (depends on F2.Sub = a+b)"
tech-stack:
  added: []
  patterns:
    - "TDD RED/GREEN per task: failing table-driven test committed before each source fix"
    - "Fold rule driver iterates a fixed-order slice (rewriteRuleOrder), never a map"
key-files:
  created:
    - internal/f2/expr_test.go
    - internal/f2/fold_test.go
    - internal/f2/coeff_mat_test.go
    - internal/f2/vector_test.go
  modified:
    - internal/f2/expr.go
    - internal/f2/fold.go
    - internal/f2/coeff_mat.go
    - internal/f2/vector.go
key-decisions:
  - "rewriteEngine map + init() removed entirely in favor of rewriteRuleOrder slice — the map had no other consumers, and keeping a dead registration map alongside the ordered slice would violate the no-dead-code convention"
  - "TestFoldNormalization and TestCoeffMatApplyZero added beyond the plan's listed behavior tests — CLAUDE.md mandates every bug fix ship with unit tests, and the plan's action items fix rewriteNormalization and Apply without enumerating direct tests for both"
patterns-established:
  - "Table-driven stdlib tests with tiny local mustEqual/mustNotEqual helpers (t.Helper()), no assertion libraries"
  - "Determinism verified by -count=20 repeated test runs"
requirements-completed: [ALG-01, ALG-02, ALG-05]
duration: ~18 min
completed: 2026-11-12
---

# Phase 01 Plan 02: Semantic Bug Fixes + Table-Driven Proofs Summary

**Fixed four provably-wrong F₂ math sites (Sub, fold normalization, matrix-vector Apply op roles, fold driver nondeterminism, Gather indexing) and locked the algebra's correctness with four stdlib-only table-driven test files — build/vet/test all green with go.mod untouched.**

## Performance

- **Duration:** ~18 min
- **Started:** 2026-11-12T12:02:40Z
- **Completed:** 2026-11-12T12:21:00Z
- **Tasks:** 3 (each TDD: RED commit then GREEN commit)
- **Files modified:** 8 (4 source, 4 test)

## Accomplishments
- F2.Sub computes a+b in F₂ (was a+b+b = a) — the scalar that matop.go's Gaussian elimination depends on
- Fold engine is deterministic: fixed-order `rewriteRuleOrder` slice replaces randomized map iteration; all six F₂ identities + constant-subtree folding proven by named table entries
- CoeffMat.Apply computes the true F₂ matrix-vector product (map=AND, reduce=XOR — roles were swapped, so the identity matrix returned x·x-style garbage)
- rewriteNormalization combines the two coefficients (was coefficient+variable): S1·x + S2·x ⇒ (S1+S2)·x
- Vector.Gather orders results by the index list (was writing at the source index; also panicked for idx ≥ len(idxList))
- ROADMAP criterion 4 smoke test: composite expression ((M·x)⊕(N·y))⊕(x·x) built, folded (x·x→x by idempotence), and walked with Visit — variables exactly {x,y}, symbols M,N untouched, no panic

## Task Commits

1. **Task 1 RED:** `328d29e` (test) — failing F2.Sub truth-table test
2. **Task 1 GREEN:** `746e8d5` (fix) — F2.Sub returns self.Add(that)
3. **Task 2 RED:** `6e62660` (test) — failing fold-normalization + Apply identity/swap/zero tests
4. **Task 2 GREEN:** `b7c1e6f` (fix) — deterministic rule order, normalization math, Apply op roles
5. **Task 3 RED:** `eaad49f` (test) — failing Gather order test + smoke test
6. **Task 3 GREEN:** `934d718` (fix) — Gather writes at range position

## Files Created/Modified

- `internal/f2/expr.go` — F2.Sub body fix (the only source change in this file)
- `internal/f2/fold.go` — rewriteRuleOrder slice replaces rewriteEngine map + init(); rewriteNormalization combines a+c
- `internal/f2/coeff_mat.go` — Apply map step Mul(AND), reduce step Add(XOR)
- `internal/f2/vector.go` — Gather writes out[i] = vec[idx]
- `internal/f2/expr_test.go` — TestF2Add/Mul/Sub truth tables, TestIsEqualStructural (ordered, non-commutative), TestIsExprZeroOne
- `internal/f2/fold_test.go` — TestFoldIdentities (10 named cases), TestFoldNormalization, TestFoldConstantSubtree, TestFoldCompositeIdempotent, TestFoldDeterministic, TestFoldNoAliasing (tree-never-DAG)
- `internal/f2/coeff_mat_test.go` — TestCoeffMatApplyIdentity/Swap/Zero
- `internal/f2/vector_test.go` — TestVectorGather, TestVectorGatherEmpty, TestSmokeFoldInspect (ROADMAP criterion 4)

## Semantic Bugs Fixed (file, before, after)

| File | Before | After |
|------|--------|-------|
| expr.go `F2.Sub` | `self.Add(that).Add(that)` = a+b+b = **a** | `self.Add(that)` = a+b (F₂: −b = +b) |
| fold.go driver | `for _, r := range rewriteEngine` — Go map order, nondeterministic canonical form | `for _, r := range rewriteRuleOrder` — fixed 7-rule order |
| fold.go `rewriteNormalization` | `coeff := NewBinary(Add, a, b)`; `Mul(coeff, c)` — pairs coefficient with variable | `coeff := NewBinary(Add, a, c)`; `Mul(coeff, b)` — pairs the two coefficients |
| coeff_mat.go `Apply` | map appends `NewBinary(Add, …)`, reduce folds with `NewBinary(Mul, …)` | map appends `NewBinary(Mul, …)`, reduce folds with `NewBinary(Add, …)` — y_i = XOR_j(A_ij AND x_j) |
| vector.go `Gather` | `out[idx] = self.vec[idx]` — wrong position, panic if idx ≥ len(idxList) | `out[i] = self.vec[idx]` — order follows index list |

## Decisions Made
- Removed the `rewriteEngine` map + `init()` entirely rather than keeping it alongside the ordered slice (plan permitted either): the map had no other consumers, and dead registration code violates project conventions.
- Added `TestFoldNormalization` and `TestCoeffMatApplyZero` beyond the plan's enumerated behavior tests: CLAUDE.md requires every bug fix ship with unit tests, and the plan's own action items fix `rewriteNormalization` and `Apply` without listing a direct behavior test for each.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added TestFoldNormalization**
- **Found during:** Task 2 (rewriteNormalization fix)
- **Issue:** Plan action #2 fixes the normalization math (a+b → a+c) but the `<behavior>` block lists no test exercising `S1*x + S2*x => (S1+S2)*x`; CLAUDE.md mandates every bug fix ship with unit tests.
- **Fix:** Added `TestFoldNormalization` to fold_test.go; verified it fails pre-fix (folds to `(S1+x)*S2`) and passes post-fix.
- **Files modified:** internal/f2/fold_test.go
- **Committed in:** 6e62660 (Task 2 RED)

**2. [Rule 2 - Missing Critical] Added TestCoeffMatApplyZero**
- **Found during:** Task 2 (Apply fix)
- **Issue:** Same class — the zero-matrix annihilation case directly exercises the swapped-operator bug but was not enumerated.
- **Fix:** Added `TestCoeffMatApplyZero`; fails pre-fix, passes post-fix.
- **Files modified:** internal/f2/coeff_mat_test.go
- **Committed in:** 6e62660 (Task 2 RED)

### Notes (non-deviation)
- F2.Sub was expanded to a multi-line body so the acceptance grep `return self.Add(that)$` matches exactly once (the plan anticipated the Sub body on its own line); no semantic difference.
- TestSmokeFoldInspect passes in the RED commit for Task 3 because it depends on Task 2's already-committed fixes; Task 3's RED requirement is satisfied by TestVectorGather's failure.

---

**Total deviations:** 2 auto-fixed (both Rule 2 — test coverage for plan-mandated bug fixes)
**Impact on plan:** No scope creep; both additions are tests for fixes the plan explicitly mandates.

## Issues Encountered

- `NewConstMat(raw).ToCoeffMat()` in the test helper failed to compile (pointer-receiver method on non-addressable literal) — assigned to a local first, matching the repair pattern already used in matop.go.
- Acceptance grep `for (key|k)... range rewriteEngine` requires zero map iteration: satisfied by removing the map rather than keeping dead registration code.

## User Setup Required

None — pure local library work.

## Known Stubs

None. Every test asserts against freshly constructed expected trees via structural equality; no placeholder or empty-value data flows anywhere.

## Verification (final gate, from repo root)

```
$ go build ./...                                # exit 0 (ALG-05)
$ go vet ./...                                  # exit 0 (ALG-05)
$ go test ./... -count=1                        # exit 0, internal/f2 ok
$ go test ./internal/f2/ -run 'TestFold|TestCoeffMatApply' -count=20   # exit 0 (determinism)
$ grep -c "testify\|require\.\|assert\." internal/f2/*_test.go   # 0 0 0 0 (stdlib only)
$ git diff go.mod                               # empty (zero new dependencies)
```

All plan `must_haves.truths` hold: construction/structural equality (TestIsEqualStructural), the six fold identities + constant folding (TestFoldIdentities), determinism (TestFoldDeterministic + -count=20), tree-never-DAG (TestFoldNoAliasing), and the smoke run (TestSmokeFoldInspect).

## Self-Check: PASSED

- All 8 source/test files exist on disk (verified above).
- All 6 task commits (328d29e, 746e8d5, 6e62660, b7c1e6f, eaad49f, 934d718) present in git log.
- Final gate green: go build ./..., go vet ./..., go test ./... -count=1 all exit 0; -count=20 determinism run exit 0; go.mod unchanged; zero third-party test imports.

## Next Phase Readiness
- Core algebra is now compile-clean (01-01) *and* semantically correct on hand-checked cases (01-02) — ALG-01/02/05 satisfied.
- Per PROJECT.md, further work (layout maps, utilities like expression dumpers/pretty printers) is user-driven and on-demand; no autonomous next phase.

---
*Phase: 01-build-fix-working-algebra-core*
*Completed: 2026-11-12*
