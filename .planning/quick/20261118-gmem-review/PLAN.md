---
type: quick
date: 2026-11-18
---

# Review + fix + test user's GMEM optimization pass (subgroupchain)

User implemented the GMEM optimization (Coordinate IR, coord analysis, family-based
swap pass). Claude's job: review, fix, add tests. User owns SMEM/swizzle next.

## Done

Fixes (compile, ~40 errors + logic):
- common: Reduce signature (undefined type params), unused imports, Slice off-by-one
  (+1 removed -> Python semantics), added sorted Keys, Min3
- expr.go: CoordInfo.Reset phantom fields, IsOnlyOccupiedByFamily missing bool,
  PartiteCoord moved to Coord level + root participation + pure-subtree-kept-whole,
  Variable embeds coordNode (satisfies Coord), Const coordNode dropped, Coordinate
  IsEqual field/typo + bound compare, CoordChildren []Coord + Binary-value leaf
  handling, Replace: NodeBinary->Binary + Stride/Coordinate descent + loop-copy
  consumed bug, IsCoord lowercase type
- visit.go/range.go: Coordinate descent + range case; MustGetUpperBound CheckNotNil
- coord_analysis.go: VisitPostOrder->VisitPost, imports, Coordinate-of-Binary
  variable-subtree scan (families from underlying Variables), sorted keys
- factory.go: uncalled closure in Check, checkCoord call, identity fast path in
  Factorize (single whole-range digit returns the input node)
- gmem_map.go: full repair preserving design — GMemCoalesce rename, undefined
  symbols (config.Shape/layout.Raw/layout.Last/FindVariableByFamily/ReplaceExpr),
  existedSize slice->product, draw exactly W/R' (was: drain whole pool — infeasible
  on valid inputs), portion==1 skip, Split call signatures, deferred replacements
  (failed run leaves layout untouched), nested-last-dim + config errors, window
  W=min(lastDim, line, V) with V scanned from the layout

Tests: coord_test.go (Coordinate/Split/AnalyzeCoord/Partite/Replace incl. nested
rewrite + consumed-once), gmem_test.go (full swap 32->4 sectors, no-op fast path,
partial window, idempotence, error paths leave layout unchanged, config errors,
nested last dim, exhaustive property test: 964 (BoV,tile) pairs N<=48 — bijection
preserved always, occupancy >= W on success, 855 optimized / 109 bailouts).
Existing tests updated for 1-element-Stride identity semantics.

## Verification

go build / go vet / go test ./... -count=1 all green; -count=5 deterministic.
