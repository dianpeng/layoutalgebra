---
phase: quick-261113-jpv
plan: 01
subsystem: fold-engine
tags: [fold, normalization, tdd, coefficient-extraction]
requires:
  - "rewriteNormalization both-Mul static-coefficient merge (pre-existing)"
  - "extractLinearCoeffVector per-variable closure extraction (vector.go, untouched)"
provides:
  - "C1*v + C2*v merge firing when one side is a bare variable (C=1), both operand orders"
  - "Silent-drop fix: CoeffMatAll on x + M*x yields merged coefficient (1+M), not M"
affects:
  - "internal/f2/fold.go rewriteNormalization"
  - "internal/f2/fold_test.go"
tech-stack:
  added: []
  patterns:
    - "Two explicit operand-order branches in one normalization rule (no reliance on rewriteCommute)"
key-files:
  created: []
  modified:
    - "internal/f2/fold.go"
    - "internal/f2/fold_test.go"
decisions:
  - "Pre-fix failure mode ground truth: silent wrong coefficient (extractor returns M), NOT a CHECK panic — plan-checker prediction corrected by direct probe"
  - "Both operand orders converge to Mul(Add(One, S), x), exactly as plan-checker simulated"
metrics:
  duration: ~5 min
  completed: 2026-11-13
---

# Quick Task 261113-jpv: Extend fold normalization to bare-variable case — Summary

Bare-variable coefficient-1 merge added to `rewriteNormalization`: `C1*v + C2*v => (C1+C2)*v` now fires when one side is a bare variable, in both operand orders, converging to the single canonical tree `Mul(Add(One, S), x)`. This fixes the `x + M*x` silent-drop in `CoeffMatAll` extraction at the fold layer, per the 261113-gll documented limitation; `vector.go` stays untouched per user direction.

## Pre-fix Failure Mode — Ground Truth (probe-established)

Two reviewer probes disagreed on the pre-fix failure mode of the certificate test. Direct probe (scratch program deleted after use) established:

- **Silent wrong coefficient.** Pre-fix `CoeffMatAll([x + M*x])` returns a 1x1 matrix whose single entry is `M` — no error, no panic. The extractor's `exprMap[name] = e` overwrite keeps only the `M*x` closure and strips the coefficient as `node.L()`.
- **Not a CHECK panic.** The plan-checker's predicted panic at the variable-kind check in `extractLinearCoeffVector` does not occur for this input shape: the overwritten closure is exactly `Mul(M, x)`, whose `R()` is a `*NodeVariable`, so both `check.Check` calls pass.
- Pre-fix `Fold(x + S*x)` returns the unfolded `Add(x, Mul(S, x))`; no normalization rule fires.

## Pinned Canonical Trees (both operand orders)

- `Fold(x + S*x)` == `Fold(S*x + x)` == `Mul(Add(One, S), x)` — matches plan-checker simulation exactly. The `S*x + x` order passes through the intermediate `Add(S, One)` and `rewriteSymbol` commutes it in place to `Add(One, S)`.
- `Fold(x + 1*x)` == `Fold(1*x + x)` == `ConstZero` — merged coefficient `Add(One, One)` folds to 0, then `Mul(0, v)` folds to 0 via rewriteFold. (This case was already green pre-fix via the identity fold; kept as a lock.)
- Negative guards pass through untouched: `x + S*y` (different variable), `x + y` (two bare variables), `x + S*(x+y)` (non-static coefficient).
- Regression locks unchanged: `M*x + N*x -> Mul(Add(M,N), x)`, `x + x -> 0`.

## Commits

| Commit | Type | Description |
|--------|------|-------------|
| `2441315` | test | RED: four new tests, failing on pre-fix code |
| `1105048` | feat | GREEN: two bare-variable branches in rewriteNormalization |

## Verification

- `go build ./...` — green
- `go vet ./...` — green
- `go test ./... -count=1` — green (all packages)
- `go test ./internal/f2/ -count=20 -run 'TestFold|TestPipeline'` — green, no `check.Bug` iteration-cap aborts (fixpoint determinism, WR-10)
- Grep gate: `NewConstOne` count in fold.go (non-comment lines) = 4 (>= 3 required); `exprMap[name] = e` count in vector.go = 1, unchanged (vector.go untouched)

## TDD Gate Compliance

- RED gate: `test(261113-jpv)` commit `2441315` — new tests confirmed failing on pre-fix code (`x_plus_Sx`, `Sx_plus_x`, certificate soundness failed; negative/const/regression subtests passed pre-fix by design as locks).
- GREEN gate: `feat(261113-jpv)` commit `1105048` — all tests pass.
- No refactor commit needed (patch is minimal).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Doc] Corrected pre-fix failure-mode prediction in test documentation**
- **Found during:** RED probe
- **Issue:** Plan (echoing plan-checker) predicted the certificate test's pre-fix failure would be "a CHECK panic at the variable-kind check in extractLinearCoeffVector"; an earlier probe had observed a silent wrong coefficient.
- **Fix:** Direct probe established the silent wrong coefficient (`M`) is the true pre-fix behavior — the overwrite leaves `Mul(M, x)` whose `R()` passes the variable-kind check, so no panic. The test was written against this truth (asserting coefficient `(1+M)`, never weakened); only the plan's prediction was wrong. Recorded here as the authoritative answer.
- **Files modified:** none (test comment states the silent-drop mechanism accurately)
- **Commit:** N/A (documentation-level correction only)

None otherwise — plan executed exactly as written; the GREEN patch matches design_pins verbatim.

## Self-Check: PASSED

- `internal/f2/fold.go` modified (GREEN branch present, grep-verified) — FOUND
- `internal/f2/fold_test.go` modified (4 new tests) — FOUND
- Commit `2441315` (RED) — FOUND
- Commit `1105048` (GREEN) — FOUND
- `internal/f2/vector.go` untouched (empty diff) — VERIFIED
