# Quick Task 261124-bn9 Summary — Alpha-Release Polish

**One-liner:** Alpha-release polish for the layoutalgebra Go library: test
orchestrator script, conservative function splits, public-API audit fixes,
verified documentation refresh, three runnable optimization examples, and
two theory docs — full suite green throughout.

## Per-Item Results

### Item 2 — scripts/test.py (Task 1, commit d7940a3)

Created `scripts/test.py`: Python 3.13, stdlib only (subprocess, argparse,
time, shutil, os), raw ANSI colors, 208 lines. Discovers packages via
`go list ./...` resolved from the script location (not CWD), runs
`go test -count=1` per package, renders one colored section per package
(PASS collapsed / FAIL full output / NO TESTS separate from PASS), prints a
summary line (package counts + wall time), supports `--verbose` and
`--pkg SUBSTRING`. Exit codes verified live: 0 on all-pass, 1 on failure
(with a deliberately broken temp test — failing package's output shown in
full), 2 when `go` is missing from PATH, 130 on keyboard interrupt.

### Item 1 — code polish (Task 2, commit 9174e96)

Measured every function body in `internal/f2` and `internal/subgroupchain`
with a throwaway script (not committed). Only two functions exceeded 100
lines; both split, no signature/algorithm/error-message/test changes:

- `internal/f2/vector.go` — `extractLinearCoeffVector` (195 → ~45 lines):
  extracted `novMarkPostOrder` (NOV marker pass), `checkLinearCoeffLinearity`,
  `forwardCollectFrontier` (variable-closure map + dangling-Mul factors),
  `distributeDanglingMul` (distribution law + fold + write-back), and
  `extractBasisCoefficient` (per-variable coefficient switch). All new
  helpers unexported with explicit parameters; original explanatory
  comments moved with their phases.
- `internal/f2/swizzle_map.go` — `swizzler.Run` (121 → 92 lines): extracted
  `bankNullspace` (bank-bit rank + nullspace indices) and `freeDimensions`
  (same-family bits above the bank range).

Post-refactor worst function body anywhere in the two packages: 92 lines
(`expandPieces` 85, `GuassianEliminationConstMat` 83 are next). Untouched
test suite stayed green after every batch; `gofmt -l .` clean.

### Item 4 — public API audit (Task 3, commit eb95f91)

Audited root package, `pkg/ast`, `pkg/layout`, `example/`. Small additive
fixes only:

- `algebra.go`: `Problem` doc now states Concurrency order is semantic and
  the sub-group chain algebra expects the lane variable LAST (the lane-last
  convention was previously only documented in an internal comment in
  `pkg/layout/subgroupchain.go`).
- `pkg/ast/print.go`: per-method godoc comments on all eleven `String()`
  methods (previously one section comment that godoc never surfaces).
- `pkg/ast/eval_test.go`: new `ExampleEval` godoc example demonstrating
  `ast.Eval`'s variable-binding contract (additive only; existing tests
  untouched).

`go vet ./...` clean (was already), `go run ./example` verified, all
exported identifiers in the public surface carry accurate godoc comments.

**Flagged, not changed (breaking-change candidates for the user):**
1. `layout.Mapping` — mildly stuttering name under full qualification
   (`layout.Mapping`); renaming is breaking, left as-is.
2. `internal/f2.GuassianEliminationConstMat` — "Guassian" typo; internal
   only, safe to rename but outside this task's public-surface scope.

### Item 3 — documentation refresh (Task 4, commit 344396d)

- **README.md**: quick-start now pairs `layout.SubgroupChain` with its
  non-pow2 example problem (it previously claimed `layout.F2` on a 7×16×4
  tile, which `NewSession` rejects) and the one-token-switch note now
  states both algebras' preconditions; the `ast.Eval` reference line became
  real code so the documented import block compiles — verified by
  extracting the block to a temp package (`go vet` + `go run` clean);
  stale test count corrected 73 → 71 (measured); new **Documentation**
  section links `doc/f2.md` and `doc/subgroupchain.md`.
- **internal/subgroupchain/README.md**: the "SMem Bank Conflicts
  Avoidance" section claimed NOT implemented — corrected to PARTIALLY
  implemented: the framework `Smem` pass exists (`smem_swizzle_map.go`),
  the unimplemented piece is the `TrySwizzle` transvection hook; old design
  notes kept below as intended-swizzle background.
- **internal/f2/theory.md**: verified against source (`folder.run` at
  fold.go:404, `TestFoldCanonicalEnumeration`, `TestFoldMeasureDecreases`,
  `TestFoldConfluence`, `OTuple` all present; Task 2 renamed nothing) — no
  edits needed.
- **doc.go**: already documents the three-layer architecture and the
  pkg/layout + pkg/ast consumer entry points accurately — no edits needed.

### Item 5 — compiler-optimization examples (Task 5, commit 4a6cac5)

Created `examples/` alongside the singular `example/` (which remains the
offset-printing / `ast.Dump` tour; the new programs cover ground it
doesn't — a reusable consumer-side pass wrapper, numeric before/after
cost-model tables, and option tuning):

- `examples/compilerpass/main.go` (140 lines) — the headline consumer-side
  "pass" pattern: wraps each stage's `ast.Expr` as an `offsetPass` with
  `at()`/`sectorTable()` evaluation; F2 lane-last (documents that F2's GMEM
  M is identity by the trailing-lane convention, so base/gmem offsets
  coincide) and sgc lane-first where the GMEM rewrite is visible
  numerically; conservative smem decline shown as contract behavior.
- `examples/gmem-coalescing/main.go` (83 lines) — sgc with
  `WithLineBytes(128)`: per-warp distinct 32B sectors and 128B lines,
  before/after: **24 sectors, 6 lines → 4 sectors, 1 line** (verified
  output).
- `examples/smem-swizzle/main.go` (118 lines) — F2 with
  `WithBankWindow(2,5)`: distinct-banks-per-warp report via `ast.Eval` over
  the lane sweep; saturated tiling (32/32 banks, stage returns unchanged)
  and conflicted tiling (8/32 banks, stage declines with the classified
  `no free degree` verdict); notes where `WithBankBytes` feeds the sgc
  backend.
- `examples/README.md` — index with run commands.

All three compile and run with exit 0 (`go build ./...` + per-example
`go run` verified).

### Item 6 — theory docs (Task 6, commit 2252216)

- `doc/f2.md` — GF(2) arithmetic, bit-vectors-as-F2-vectors, the matrix
  view and what "exact" means vs sgc conservatism; the hardware-bits →
  offset-vector model and why the power-of-2 restriction exists; a worked
  `[4,8]`-tile example with verified program output (offset AST, full
  32-point enumeration, identity-M gmem, smem decline); guarantees table
  citing `canon_test.go`, `convergence_test.go`, `layout_cuda_test.go`,
  `pipeline_test.go`, `gmem_coalescing_map_test.go`, and the pkg/layout
  lowering machine-checks; use cases and out-of-scope boundaries.
- `doc/subgroupchain.md` — Z/k sub-group chains and mixed-radix digits,
  HTuple coordinate model, BoV access mapping (gcd low-digit peeling),
  stride mapping, Coordinate/Stride IR; where conservatism enters (loud
  soundness invariants, classified pass failures, monotone interval
  analysis); worked `[2,4]`-tile example with verified output; guarantees
  table citing `soundness_test.go` (1400 pairs), `map_test.go`,
  `gmem_test.go` (1794 cases), `smem_test.go` (1021 cases), `eval_test.go`;
  pending-work boundary (`TrySwizzle` transvection, folding engine,
  reshape passes).
- Both state prerequisites, end with a "Which algebra?" decision section,
  and cross-link each other, the README, and the internal theory files.

## Deviations from Plan

1. **[Plan adjustment — Task 5 scope]** The plan asked the compilerpass
   example to show "the coalescing improvement" via `layout.F2`. Measured
   behavior: F2's `GmemCoalescingMap` hard-codes M = identity under the
   trailing-lane convention (`internal/f2/gmem_coalescing_map.go:258-265`),
   so F2 base and gmem offsets always coincide through the facade; the
   visible rewrite is the sgc pass's family-swap behavior. The example runs
   the F2 three-stage skeleton as instructed and demonstrates the numeric
   before/after on the sgc backend, with comments explaining why (verified:
   F2 lane-first also returns M = identity). No aspirational claims added.
2. **[Rule 2 — docs correctness]** README quick-start paired `layout.F2`
   with a non-pow2 tile and imported `pkg/ast` without using it; both would
   fail for a copying consumer. Fixed in place (quick-start switched to
   SubgroupChain / eval line made real); documented in Task 4 commit.
3. **[Docs-only corrections]** README test count 73 → 71; subgroupchain
   README SMEM status corrected from "NOT implemented" to the accurate
   partial-implementation statement. Both verified against source.

No auth gates, no checkpoints, no stub patterns introduced.

## Verification Outputs (final state)

```
$ python3 scripts/test.py
11 package(s) | 7 passed | 0 failed | 4 skipped        exit=0
   (4 skipped = example/, examples/*, tmp dirs with no test files)

$ go vet ./... && go build ./...                         clean

$ gofmt -l .                                             (empty)

$ go run ./example && go run ./examples/compilerpass \
    && go run ./examples/gmem-coalescing && go run ./examples/smem-swizzle
                                                          all exit 0

$ grep sweep: every exported identifier named in README/doc.go/docs
  exists in source (func signatures verified individually)
```

## Commits

| Task | Commit | Message |
|---|---|---|
| 1 | d7940a3 | chore(261124-bn9): add scripts/test.py per-package Go test orchestrator |
| 2 | 9174e96 | refactor(261124-bn9): split oversized functions in internal/f2 |
| 3 | eb95f91 | docs(261124-bn9): public API sanity audit — additive doc fixes only |
| 4 | 344396d | docs(261124-bn9): refresh README + package docs against actual code |
| 5 | 4a6cac5 | feat(261124-bn9): add examples/ compiler-pass, GMEM, and SMEM programs |
| 6 | 2252216 | docs(261124-bn9): add doc/f2.md and doc/subgroupchain.md theory docs |
