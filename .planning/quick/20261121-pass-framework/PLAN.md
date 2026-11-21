---
type: quick
date: 2026-11-21
---

# GMEM/SMEM universal optimization framework refactor

User request: refactor the GMEM process into a general framework usable as the
backbone for both GMEM and SMEM bank-conflict resolution, with the abstracted
logic as universal, independently verifiable components.

## Structure

Universal components (each independently testable):
- eval.go: production reference interpreter (Eval) — backs sweep predicates
- sweep.go: SweepLane + SweepStats — one warp sweep, both objectives
  (sectors AND bank distinctness), Contiguous/BankConflictFree predicates
- lastdim.go: LastDimAnalysis + AnalyzeLastDim + FamilyFreedom (+ flatten /
  digit-run decomposition / algebraic flattening / classify)
- repair.go: coverage repair engine — ReorderLastDim + DrawIntoLastDim
  (divisor search, gcd theorem, transactional via tentative remap +
  pre-commit suffix verification), assembleLastDim (shape-dependent order)
- pass.go: Pass interface (Family / ByteWindow / ObjectiveSatisfied /
  TryMappingRepair / Validate), Context (with Sweep), Optimize driver
  (window -> analysis -> validate -> fast path -> transform stage ->
  coverage repair -> post-check)

Instantiations:
- gmem_map.go: GMemCoalesce — suffix-coverage objective, no transform stage
  (GMEM entry signature unchanged)
- smem.go: SmemCoalesce + SmemSwizzle hook (stage-4 extension point for the
  user's transvection), sweep-distinctness fast path, coverage-draw fallback,
  unique-family-variable validation

## Findings during the refactor (both real bugs fixed)

- Nested last dims: the sub-access cut boundaries can split the family block
  — the assembly order alone does not guarantee the suffix. The old code
  committed the repair and (without a post-check) reported false successes.
  Now the remap is built tentatively (AccessWithCoord is pure), the suffix
  verified, and only then committed — full transactionality restored; the
  previously silent cases become classified "not representable in the
  factorization" errors (5 exposed by the post-check, ~17 more by pre-commit
  verification).
- Multi-variable families silently neutered SMEM's sweep predicate (Lane=nil
  -> suffix fallback -> success while the swept variable still collided).
  Now rejected by Validate with a clear error.

## Numbers

- GMEM property: 1794 cases, 1432 verified successes / 362 classified
  bailouts (all layout-untouched; previously 1454 included ~22 unverified/
  false successes)
- SMEM property (new): 1021 single-lane cases, 845 conflict-free successes,
  every success machine-checked for bijection + distinct banks
- go build / vet / test ./... green; -count=5 deterministic

## Boundary kept

The swizzle transvection itself is NOT implemented — TrySwizzle is the
documented hook (user-owned per the authorship arrangement). For a unique
family variable, coverage implies conflict-freedom under the one-warp model,
so the hook is required only for unrepresentable windows (nested cuts) and
copy-pattern-sensitive cases.
