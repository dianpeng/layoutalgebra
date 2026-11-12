---
phase: 01-build-fix-working-algebra-core
verified: 2026-11-12T00:00:00Z
status: passed
score: 9/9 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: none
  previous_score: n/a
  gaps_closed: []
  gaps_remaining: []
  regressions: []
---

# Phase 01: Build Fix & Working Algebra Core Verification Report

**Phase Goal:** The whole module builds and vets clean, and the F₂ expression tree with folding behaves correctly on hand-checked cases.
**Verified:** 2026-11-12
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `go build ./...` exits 0 on the whole module (packages f2, check, common) | ✓ VERIFIED | Ran from repo root: BUILD_EXIT=0; all three packages compile |
| 2 | `go vet ./...` exits 0 on the whole module | ✓ VERIFIED | Ran from repo root: VET_EXIT=0, zero diagnostics |
| 3 | go.mod unchanged — zero new dependencies | ✓ VERIFIED | `git diff --stat -- go.mod go.sum` empty; go.mod contains only module path + `go 1.26.2`; no go.sum exists |
| 4 | User can construct Variable/Const/Symbol/Xor/And expressions and compare with structural equality (ordered, operator-sensitive) | ✓ VERIFIED | TestIsEqualStructural passes incl. `mustNotEqual(NewAdd(x,y), NewAdd(y,x))` (order matters) and operator-matters case; IsEqual implementations inspected — NodeBinary compares op + ordered operands (expr.go:169-176), NodeVariable/NodeConst/NodeSymbol are name/value-structural |
| 5 | Folding reduces constants and redundant ops: x⊕0→x, 0⊕x→x, x⊕x→0, x·0→0, x·1→x, x·x→x, plus constant-only subtree folding | ✓ VERIFIED | TestFoldIdentities (10 named table cases incl. all six identities + const folding) + TestFoldConstantSubtree + TestFoldCompositeIdempotent all pass; tests assert against freshly constructed expected trees via IsEqual — discriminating (a no-op fold would fail `xor_x_x_to_0` since NewAdd(x,x) ≠ NewConstZero structurally) |
| 6 | Folding is deterministic — same expression folds to structurally equal results across runs | ✓ VERIFIED | `rewriteRuleOrder` fixed-order slice (fold.go:61) iterated by the driver (fold.go:369); zero `range rewriteEngine` map iteration; `go test -run 'TestFold\|TestCoeffMatApply' -count=20` exits 0; TestFoldDeterministic passes |
| 7 | tree-never-DAG invariant holds through fold (rewrite but no silent elimination/sharing of unrelated nodes) | ✓ VERIFIED | TestFoldNoAliasing passes: Fold(Xor(0,v)) IsEqual v and variable set exactly [v] via GetExprVariableNameSet |
| 8 | Smoke run builds a composite expression, folds it, inspects without crashing or mis-printing (ROADMAP SC 4) | ✓ VERIFIED | TestSmokeFoldInspect passes: ((M·x)⊕(N·y))⊕(x·x) folds to Xor(Xor(M·x, N·y), x) (idempotence), Visit walk yields variables {x,y}, symbols [M N], no panic |
| 9 | `go test ./...` exits 0; tests are stdlib-only table-driven | ✓ VERIFIED | TEST_EXIT=0 (17 tests pass); `grep -c "testify\|require\.\|assert\." internal/f2/*_test.go` → 0 in all 4 files |

**Score:** 9/9 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/f2/expr.go` | Expression tree types, structural IsEqual, Replace, Symbolize | ✓ VERIFIED | 296 lines; IsEqual implementations inspected and correct |
| `internal/f2/visit.go` | Visit/VisitPost/VisitBy traversal | ✓ VERIFIED | 96 lines; used by smoke test and tests, wired |
| `internal/f2/fold.go` | Fold engine, entry point `Fold(Expr) Expr` | ✓ VERIFIED | 424 lines; `func Fold(expr Expr) Expr` at fold.go:421; deterministic rule order at fold.go:61,369 |
| `internal/f2/swizzle_map.go` | Compiling swizzle map + name-family helpers | ✓ VERIFIED | 235 lines; compiles, helpers present (NameBelong swizzle_map.go:40, BelongVariableNameFamily swizzle_map.go:46) |
| `internal/f2/coeff_mat.go` | Coefficient matrix, Apply with correct F₂ map/reduce roles | ✓ VERIFIED | Apply: map=Mul(AND) coeff_mat.go:186, reduce=Add(XOR) coeff_mat.go:190 — the true y_i = XOR_j(A_ij AND x_j) |
| `internal/f2/vector.go` | Vector incl. Slice/Set/Gather/GetExprVariableNameSet | ✓ VERIFIED | Gather writes at range position (vector.go:29); Slice:38, Set:45, GetExprVariableNameSet:178 all present |
| `internal/f2/expr_test.go` | min 60 lines | ✓ VERIFIED | 130 lines; 4 test functions, truth tables over {0,1}² |
| `internal/f2/fold_test.go` | min 80 lines | ✓ VERIFIED | 119 lines; 6 test functions |
| `internal/f2/coeff_mat_test.go` | min 40 lines | ✓ VERIFIED | 71 lines; Identity/Swap/Zero tests |
| `internal/f2/vector_test.go` | min 40 lines | ✓ VERIFIED | 112 lines; Gather order/empty + smoke test |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| fold.go | expr.go Replace | `Replace(expr` in driver | ✓ WIRED | fold.go:416 `expr = Replace(expr, replacementQueue)` |
| coeff_mat.go Apply | vector.go NewVectorWithFold | folded vector return | ✓ WIRED | coeff_mat.go:198 `return NewVectorWithFold(v)` |
| swizzle_map.go | vector.go Vector methods | Slice/Set/Gather/Index calls | ✓ WIRED | swizzle_map.go:131 Slice, :179/:221 Index, :188 Gather, :227 Set |
| fold.go rewriteEngine | ordered rule list | fixed-order slice, no map iteration | ✓ WIRED | `rewriteRuleOrder` slice (fold.go:61) is the only iteration source (fold.go:369); `for.*rewriteEngine` grep = 0 matches |
| coeff_mat.go Apply | fold.go Fold | NewVectorWithFold folds each row | ✓ WIRED | NewVectorWithFold (vector.go:458) → Vector.Fold (vector.go:47) → `Fold(expr)` per element |
| expr.go F2.Sub | matop.go subRawFrom | row elimination uses Sub as F₂ addition | ✓ WIRED | expr.go:200 `return self.Add(that)` (a+b in F₂); matop.go:92 `dst[idx] = l.Sub(r)` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| fold.go Fold | expr tree | caller-constructed tree, rewritten via Replace | ✓ real (no static/empty returns) | ✓ FLOWING |
| coeff_mat.go Apply | per-row reduced exprs | self.Index/that.Index of real matrix/vector cells → Mul map + Add reduce → NewVectorWithFold | ✓ real | ✓ FLOWING |
| Test files | expected trees | freshly constructed per case via NewVariable/NewConst/NewSymbol factories | ✓ real (discriminating assertions, not mirrors of implementation) | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Whole module builds | `go build ./...` | exit 0 | ✓ PASS |
| Whole module vets | `go vet ./...` | exit 0 | ✓ PASS |
| Full test suite | `go test ./... -count=1` | ok internal/f2; exit 0 | ✓ PASS |
| Fold/Apply determinism | `go test ./internal/f2/ -run 'TestFold\|TestCoeffMatApply' -count=20` | ok; exit 0 | ✓ PASS |
| All 17 tests individually | `go test ./internal/f2/ -v -count=1` | 17/17 PASS, 0 FAIL | ✓ PASS |
| No dependency drift | `git diff --stat -- go.mod go.sum` | empty | ✓ PASS |
| Stdlib-only tests | `grep -c "testify\|require\.\|assert\." internal/f2/*_test.go` | 0/0/0/0 | ✓ PASS |

### Probe Execution

Step 7c: SKIPPED — no probe scripts declared in PLANs and no conventional `scripts/*/tests/probe-*.sh` exist (pure Go library phase; the go build/vet/test gate above is the runnable check).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| ALG-05 | 01-01, 01-02 | `go build ./...` and `go vet ./...` pass clean on whole module | ✓ SATISFIED | Both exit 0 (run from repo root during this verification) |
| ALG-01 | 01-02 | Expression tree (Variable/Const/Symbol/Binary over Xor·And) with structural equality + tree-never-DAG invariant | ✓ SATISFIED | TestIsEqualStructural (ordered, operator-sensitive, cross-kind); TestFoldNoAliasing; IsEqual implementations inspected at expr.go:143-193 |
| ALG-02 | 01-02 | Folding preserves tree structure while reducing constants/redundant ops | ✓ SATISFIED | TestFoldIdentities (six identities + const folding), TestFoldConstantSubtree, TestFoldCompositeIdempotent, TestFoldNormalization, TestFoldDeterministic |

**Orphaned requirements:** None. REQUIREMENTS.md traceability maps exactly ALG-01, ALG-02, ALG-05 to Phase 1; all three are claimed in plan frontmatter (`requirements:` fields) and satisfied above. ALG-03/ALG-04, MAP-01..03, TEST-01/02, UTIL-01..04 are deliberately unmapped ("on-demand via /gsd:quick" per REQUIREMENTS.md and the single-phase ROADMAP) — not gaps for this phase.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | No TBD/FIXME/XXX markers, no placeholder text, no stub returns, no hardcoded empty data flowing to output in any phase file | — | Empty slice literals (`[]Expr{}`) are loop accumulators, not stubs; `return nil, nil` in fold rules is the legitimate "rule declined" path |

### Out-of-Scope Follow-Up Debt (from 01-REVIEW.md — NOT phase-01 gaps)

The advisory code review (01-REVIEW.md, 7 critical + 15 warning + 12 info) found the matrix/elimination/swizzle layer broken: `CoeffMat.ToConst` never appends rows (CR-01), Gaussian elimination rank inverted (CR-02) + stale pivot row after swap (CR-03), `extractLinearCoeffVector` panics on `S*x` / bare-variable shapes (CR-04/05), `Vector.ShiftL` wrong position math (CR-06), swizzle free-vector indexing out of range (CR-07), plus warnings WR-01..15. All of these belong to ALG-03/ALG-04 (vector/matrix layer) and MAP-01..03 (layout maps), which REQUIREMENTS.md and ROADMAP.md explicitly defer to on-demand `/gsd:quick` work. The review itself confirms the in-scope layer — "the Fold engine, F2 scalar ops, Gather/Slice, and the Apply map/reduce roles are correct and well-tested." One fold-engine warning (WR-10: driver discards rule errors and silently gives up at the 10000-iteration cap) touches in-scope code but does not affect any hand-checked case (all tested inputs reach fixpoint far below the cap); recorded here as follow-up debt for the next `/gsd:quick` session, not a phase-01 gap.

### Human Verification Required

None. The phase goal is fully programmatically verifiable for a pure Go library: build/vet/test gates, structural equality semantics, fold identity and determinism behavior, and the smoke run were all exercised directly. No UI, external service, real-time behavior, or visual judgment is involved.

### Gaps Summary

No gaps. All 9 observable truths verified against the actual codebase (not SUMMARY claims): every gate command was re-run in this verification process, every key link was traced to a concrete line, every test file was inspected for substance, and IsEqual/fold/Apply/Sub implementations were read to confirm the semantics the tests assert.

---

_Verified: 2026-11-12_
_Verifier: Claude (gsd-verifier)_
