---
phase: quick-261113-jpv
verified: 2026-11-13T00:00:00Z
status: passed
score: 6/6 must-haves verified
overrides_applied: 0
---

# Quick Task 261113-jpv: Extend fold normalization to bare-variable case — Verification Report

**Task Goal:** Extend fold normalization rule to cover the bare-variable coefficient-1 special case (C1*v + C2*v => (C1+C2)*v when C1=1 or C2=1), fixing the x + S*x silent-drop in coefficient extraction.
**Verified:** 2026-11-13
**Status:** passed
**Re-verification:** No — initial verification (with explicit merge-integrity scrutiny per task instructions)

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | Fold(x + S*x) and Fold(S*x + x) both produce Mul(Add(One, S), x) | VERIFIED | Independent scratch probe (run in throwaway module dir, cleaned up): both orders IsEqual want=true, equal to each other; TestFoldNormalizationBareVariable/x_plus_Sx and /Sx_plus_x PASS |
| 2 | Guards hold: different-variable, two-bare-variables, non-static-coefficient shapes untouched | VERIFIED | Probe: x+S*y, x+y, x+S*(x+y) all IsEqual their inputs; TestFoldNormalizationBareVariableNegative (3 subtests) PASS |
| 3 | CoeffMatAll([x + M*x]) yields single coefficient = merged (1+M), not silently M | VERIFIED | Independent probe: mat 1x1, entry string "(1 ^ M)", IsEqual Add(One,M)=true, IsEqual bare M=false; TestFoldNormalizationCertificateSoundness PASS. Pre-fix RED commit reproduces the silent-drop (see RED gate below) |
| 4 | Constant coefficient-1 merge folds fully: x + 1*x == 0 both orders | VERIFIED | TestFoldNormalizationBareVariableConst/x_plus_1x and /1x_plus_x PASS |
| 5 | Regression locks: M*x + N*x -> Mul(Add(M,N),x); x + x -> 0 | VERIFIED | Probe + TestFoldNormalizationBareVariable/Mx_plus_Nx and /x_plus_x PASS; TestFoldNormalization (pre-existing) PASS |
| 6 | Fixpoint determinism, no rule oscillation under -count=20 | VERIFIED | go test ./internal/f2/ -count=20 → ok, no check.Bug iteration-cap aborts |

**Score:** 6/6 truths verified

### Merge Integrity (per task's explicit merge-context instruction)

The branch merged AFTER the user's own commits (repo-wide gofmt via scripts/format.sh at be4494c, and the user's duplicate-closure compensation in extractLinearCoeffVector at 29e142d). Merge commit d2b8fb3 inspected directly:

| Check | Method | Result |
| ----- | ------ | ------ |
| fold.go retains the two new branches | git diff d2b8fb3^1..d2b8fb3 -- internal/f2/fold.go | +24 lines, exactly the (2) v+c*v and (3) c*v+v branches after the both-Mul block; nothing else |
| fold.go retains user's gofmt formatting | git diff -w d2b8fb3^2..d2b8fb3 -- internal/f2/fold.go | whitespace-only change (one extra blank line removed) |
| vector.go compensation untouched by merge | git diff -w d2b8fb3^1..d2b8fb3 -- internal/f2/vector.go | EMPTY — byte-identical to user's side |
| vector.go executor-side content intact | git diff -w d2b8fb3^2..d2b8fb3 -- internal/f2/vector.go | only gofmt reformatting + the user's compensation block (if existed/hasExisted → NewBinary(parent.Op(), existed, e)); nothing lost |
| Layer composition sensible | vector.go line 254: Fold(SymbolizeExcept(expr, basis)) runs BEFORE extractLinearCoeffVector (WR-01); probe shows the merged entry is a bare Add node, i.e. the fold layer fully absorbed x + M*x and the compensation stayed dormant as the residual safety net | coherent |

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| internal/f2/fold.go | rewriteNormalization with both bare-variable branches, static-coefficient guard, IsStaticExprRecursive | VERIFIED | Branches at lines 297-314; guards: Kind()==Variable, Mul Op, IsStaticExprRecursive(coeff), mul.R().IsEqual(bare); fresh-node construction (no in-place mutation); rewriteRuleOrder unchanged |
| internal/f2/fold_test.go | TestFoldNormalizationBareVariable* + certificate-soundness lock | VERIFIED | 4 new tests present (lines 53-163), all PASS |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| rewriteNormalization | rewriteRuleOrder driver | new cases in same Rewrite method, order unchanged | WIRED | rewriteRuleOrder slice (fold.go:67-75) unchanged vs both parents |
| extractLinearCoeffVector | Fold | doCoeffMat folds before extraction (WR-01) | WIRED | vector.go:254 Fold call present and exercised by certificate test |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Certificate lock: CoeffMatAll([x+M*x]) entry = (1+M) | scratch probe (go run, cleaned up) | entry "(1 ^ M)", IsEqual Add(One,M)=true, IsEqual M=false | PASS |
| Both operand orders converge | scratch probe | both IsEqual Mul(Add(One,S),x), equal to each other | PASS |
| Guards pass through untouched | scratch probe | 3 negative shapes IsEqual input | PASS |
| Regressions unchanged | scratch probe | M*x+N*x and x+x shapes correct | PASS |
| Full build/vet | go build ./... && go vet ./... | both exit 0 | PASS |
| Formatting | gofmt -l internal/ | empty output | PASS |
| Full suite | go test ./... -count=1 | ok (internal/check, internal/f2) | PASS |
| Determinism | go test ./internal/f2/ -count=20 | ok | PASS |
| Grep gates | NewConstOne non-comment count=4 (>=3); exprMap[name] = e count=1 unchanged | PASS |

### RED→GREEN Gate (verified independently, not trusted from SUMMARY)

- Ancestry: git merge-base --is-ancestor confirms 2441315 (test) < 1105048 (feat) < 4695cdb (summary) on the merged branch.
- RED genuineness: temp worktree at 2441315 (test-only commit, before GREEN) run directly: TestFoldNormalizationBareVariable/{x_plus_Sx,Sx_plus_x} FAIL, TestFoldNormalizationCertificateSoundness FAIL with "got structurally non-equal to (1 ^ M)" — reproducing the silent-wrong-coefficient failure mode the SUMMARY documented as ground truth. Worktree removed after the check.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| ALG-03 | 261113-jpv-PLAN | F₂ vector representation — one expression per output bit, with vector-level ops | SATISFIED | CoeffMatAll correctness is core to the vector representation; the silent-drop fix is proven by the certificate lock; REQUIREMENTS.md marks ALG-03 Complete |

### Anti-Patterns Found

None. No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER markers in fold.go or fold_test.go. No stub returns, no hardcoded empty data. The pre-existing `exprMap[name] = e` overwrite in vector.go is now guarded by the user's compensation branch (not a silent drop).

### Human Verification Required

None. Pure library code with complete automated coverage; no UI/visual/external-service surface.

### Gaps Summary

No gaps. The merge result (not just the branch intent) was verified: both the user's gofmt+compensation side and the executor's fold-change side are present and coherent in the merged tree, all six must-have truths hold under independent probing, the RED→GREEN TDD gate is genuine, and every plan-declared gate (build/vet/gofmt/test/-count=20/grep) passes.

---

_Verified: 2026-11-13_
_Verifier: Claude (gsd-verifier)_
