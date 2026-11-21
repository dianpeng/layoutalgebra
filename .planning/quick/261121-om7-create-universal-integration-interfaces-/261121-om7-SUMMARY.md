---
phase: quick-261121-om7
plan: 01
subsystem: universal-integration
tags: [ast, facade, registry, lowering, codegen-interchange]
requires:
  - internal/f2 native pipeline (MapAccess/StrideMap/GmemOptimizeWith/SmemOptimize)
  - internal/subgroupchain native pipeline (Access/StrideMap/GMem/Smem/Eval)
  - root layoutalgebra Problem/Mapping/Offset + registry
provides:
  - pkg/ast sealed 11-node universal AST with Eval/Children/Walk/String/Dump
  - pkg/layout Engine/Session/Mapping abstract factory for both algebras
  - root-registry adapters for f2 and subgroupchain (init-registered on import)
  - LowerAccess/LowerOffset registry-artifact lowering into pkg/ast
affects:
  - README.md (universal-integration section, architecture, project layout, status)
  - doc.go (layer-3 bullet)
tech-stack:
  added: []
  patterns:
    - sealed Expr interface (unexported isExpr marker) with pointer constructors
    - opaque artifact structs traveling in layoutalgebra Mapping/Offset Value
    - deep-copy-before-native-mutation (HTuple.Clone) for immutable adapters
    - full-assignment-space numeric equivalence testing (AST eval == native eval)
key-files:
  created:
    - pkg/ast/ast.go
    - pkg/ast/eval.go
    - pkg/ast/print.go
    - pkg/ast/ast_test.go
    - pkg/ast/eval_test.go
    - pkg/layout/layout.go
    - pkg/layout/f2.go
    - pkg/layout/subgroupchain.go
    - pkg/layout/lower.go
    - pkg/layout/layout_test.go
    - pkg/layout/f2_test.go
    - pkg/layout/subgroupchain_test.go
  modified:
    - README.md
    - doc.go
decisions:
  - One structural ast.Tuple serves both f2 MSB-first bit vectors and sgc hierarchical coordinate tuples
  - Facade holds its backend directly (no registry indirection); root adapters exist separately and use default windows
  - f2 offset vector lowers to a weighted Add/Mul chain over distinct powers of two (bits never carry)
  - sgc Coordinate lowers to Div(Mod(src, radix), stride); empty (rank-1) Stride lowers to Const(0)
  - Registry-path adapters carry per-stage native artifacts (f2Artifacts/sgcArtifacts, f2Offset/sgcOffset) interpreted only by pkg/layout
metrics:
  duration: ~16 min
  completed: 2026-11-21
  tasks: 3
  files-created: 12
  files-modified: 2
  new-tests: 42 (pkg/ast + pkg/layout)
---

# Phase quick-261121-om7 Plan 01: Universal Integration Interfaces Summary

**One-liner:** Layer-3 universal integration — `pkg/ast` (sealed 11-node
compiler-interchange AST with reference evaluator) and `pkg/layout`
(Engine/Session abstract factory over both algebras, init-registered root
adapters, per-algebra lowering to `ast.Expr`), with full-assignment-space
numeric-equivalence and bijection tests; `algebra.go` and `internal/` untouched.

## What Was Built

### Task 1 — pkg/ast (commit 55c0b44)

- Sealed `Expr` interface (unexported `isExpr()` marker) with exactly the
  eleven node kinds: `Const`, `Symbol`, `Var{Name,Family}`, `Xor`, `And`,
  `Add`, `Sub`, `Mul`, `Div`, `Mod`, `Tuple{Elem}`.
- Constructors (pointer-returning; `NewTuple` copies its slice), `Children`,
  pre-order `Walk` with subtree-cut semantics, `Eval` (missing assignment,
  div/mod-by-zero after operand evaluation, Tuple-has-no-scalar errors),
  fully-parenthesized infix `String()` and multi-line `Dump()`.
- TDD: `ast_test.go` + `eval_test.go` written first and confirmed failing
  (compile-RED), then implemented to green.

### Task 2 — pkg/layout facade + f2 backend (commit 2d68be6)

- `layout.go`: `Kind` (F2/SubgroupChain), options (`WithLineBytes`,
  `WithBankBytes`, `WithBankWindow`; CUDA defaults 128/128/(2,5)) validated
  in `New`, `Engine`/`Session`/facade `Mapping{Algebra, Access, Offset}`,
  internal `backend` interface. `NewSession` gates `Problem.Validate` then
  `Supported`. Every stage method rebuilds the native pipeline fresh.
- `f2.go`: power-of-2 + rank-equality `Supported`; bit naming `v.Name+significance`;
  MSB-first hw vector over Concurrency order (lane-last convention documented);
  dtype exponent conversion (bytes 1/2/4 → 0/1/2); identity tiling for
  Access/SMEM; `f2Artifacts`/`f2Offset` opaque payloads; `algebraF2` root
  adapter registered in `init()`.
- `lower.go` f2 half: node switch (`NodeVariable→Var`, `NodeConst→Const`,
  `NodeSymbol→Symbol`, `Xor`/`And` only), MSB-first Tuple, offset vector →
  Σ bit·2^weight Add/Mul chain; public `LowerAccess`/`LowerOffset` dispatch
  keyed on the Algebra name with typed-payload guards.
- Tests (f2_test.go): native reference evaluator (test-only), 32-assignment
  access/gmem/smem equivalence sweeps vs `f2.MapAccess+StrideMap`,
  `f2.GmemOptimizeWith`, `f2.SmemOptimize`; access bijection onto the 32
  multiples of 4 in [0,128); registry adapter chain Access→GmemOptimize→
  StrideMap numerically identical to facade + native; dispatch error cases.

### Task 3 — subgroupchain backend + docs (commit c23a066)

- `subgroupchain.go`: product-equality `Supported` (non-power-of-2 accepted);
  BoV in the GIVEN order (lane-last documented), flat `NewConstHTuple` tile,
  byte dtypeSize for StrideMap (no exponent conversion),
  `HTuple.Clone` deep-copy before the in-place `GMem`/`Smem` passes;
  `algebraSubgroupChain` root adapter registered in `init()`.
- `lower.go` sgc half: `Variable→Var{Name,Family}`, Binary op switch,
  `Coordinate→Div(Mod(src, Const(radix)), Const(stride))`,
  `Stride→` left-folded `Add` of `Mul(coord_i, Const(stride_i))` with the
  empty rank-1 Stride → `Const(0)`; `lowerHTuple`/`lowerSGCTuple` → nested
  `ast.Tuple`.
- Tests (subgroupchain_test.go): 448-assignment access + gmem equivalence on
  the plan's [7,16,4] problem; a draw-path gmem problem ([2,96], lane-first
  BoV) where the mapping genuinely changes; smem equivalence + bijection;
  conservative error propagation ([6,8] swizzle-required verdict, dual-lane-
  family validation); registry-artifact immutability across Gmem/Smem
  (deep-copy proof); rank-mismatch rejection. Plus a SubgroupChain
  end-to-end smoke test in layout_test.go using the usage-snippet problem.
- Docs: README gained the "Universal integration (pkg/ast + pkg/layout)"
  section (usage snippet, option table, stage-artifact semantics), updated
  architecture layer 3 / diagram / project layout / status; doc.go's layer-3
  bullet now points at pkg/layout + pkg/ast (comment-only edit).

## Verification Results

- `go build ./... && go vet ./...` — clean.
- `go test ./... -count=1` — whole repo green (root, internal/check,
  internal/common, internal/f2, internal/subgroupchain, pkg/ast, pkg/layout);
  42 new tests in pkg/.
- Numeric-equivalence sweeps pass over FULL assignment spaces: f2 access/gmem
  (32), f2 smem (128), sgc access/gmem (448), sgc gmem-draw (192), sgc smem (96).
- Bijection checks: f2 access offsets = all 32 multiples of 4 in [0,128);
  sgc offsets biject onto dtype multiples of [0, space·dtype) on every
  enumerated problem/stage.
- Registry: importing pkg/layout registers both adapters; `Lookup("f2")` and
  `Lookup("subgroupchain")` succeed; root `algebra_test.go` still green (no
  duplicate-name panic).
- `./scripts/format.sh` clean; `gofmt -l pkg/` empty.
- `git diff be53091 --stat -- algebra.go internal/` is EMPTY — add-only
  discipline held (no modification to algebra.go or internal/).

## Deviations from Plan

### Adaptations (plan-consistent, no user decision needed)

**1. [Test-fixture adaptation] sgc SMEM equivalence problem**
- **Found during:** Task 3 test design (verified empirically against the
  native pipeline before writing the tests).
- **Issue:** The plan's prescribed [6,8] dtype-2 problem makes the native
  `Smem` pass fail conservatively ("coverage repair could not satisfy the
  objective; the swizzle stage is required") — its bank conflicts survive
  the coverage repair. There is no successful-smem outcome to check
  equivalence on.
- **Fix:** [6,8] became the error-propagation test (which the plan also
  lists: "native conservative errors propagate as errors"), and a
  conflict-free [3,32] dtype-4 problem (32 distinct banks over a contiguous
  lane sweep) carries the equivalence + bijection checks.
- **Files:** pkg/layout/subgroupchain_test.go.
- **Commit:** c23a066.

**2. [Test-coverage addition] f2 SMEM equivalence problem**
- **Found during:** Task 2 test design.
- **Issue:** Under the adapter's mandated identity tiling, the f2 SMEM stage
  can only saturate or error (the swizzle's free-degree search needs
  rectangular patterns the universal facade cannot express). The plan's
  [4,8] access problem has no free lane bits above the bank window, so
  `f2.SmemOptimize` returns the native no-free-degree error there.
- **Fix:** The smem equivalence sweep uses a [4,32] dtype-4 problem (2 warps
  x 64 lanes; bank window (2,5) sits inside the lane-bit run → clean
  saturation), and the [4,8] problem asserts error propagation.
- **Files:** pkg/layout/layout_test.go (fixture), pkg/layout/f2_test.go.
- **Commit:** 2d68be6.

**3. [Test-coverage addition] sgc gmem draw-path problem**
- **Issue:** On the plan's [7,16,4] problem (lane-last BoV) the native GMem
  pass fast-paths (suffix already covers W), so the equivalence test would
  never exercise a mapping that actually changes.
- **Fix:** Added a [2,96] dtype-4 problem with a lane-FIRST BoV where the
  pass performs a real draw (verified: last dimension changes, AST ≠ base,
  bijection + native equivalence still hold).
- **Files:** pkg/layout/subgroupchain_test.go.
- **Commit:** c23a066.

**4. [Process] TDD commit granularity**
- **Issue:** The plan marks tasks `tdd="true"` (RED commit, then GREEN
  commit), but the orchestrator's constraints require the full
  build/vet/test gate to pass before EVERY commit and one atomic commit per
  task — a failing RED suite cannot be committed under that gate.
- **Fix:** Each task ran RED (tests written first and confirmed failing —
  compile failure for pkg/ast, not-implemented stubs for pkg/layout) and
  GREEN (implementation) locally, then landed as ONE atomic `feat` commit
  per task. No TDD step was skipped; only the intermediate commit was folded.
- **Commits:** 55c0b44, 2d68be6, c23a066.

### Auto-fixed Issues

None beyond the adaptations above — no code defects were discovered mid-task.

## Known Stubs

None. All shipped functionality is real and machine-checked.

## Threat Flags

None. The mitigations from the plan's threat register are in place:
`NewSession` validates the Problem and the algebra's Supported check before
any native call; options are validated in `New`; `LowerAccess`/`LowerOffset`
and the adapter entry points type-switch on the payload with default errors
and never panic on unknown `Value` kinds.

## Self-Check: PASSED

- Files: all 12 created files and 2 modified files verified present on disk.
- Commits: 55c0b44 (pkg/ast), 2d68be6 (facade + f2), c23a066 (sgc + docs)
  verified in `git log`.
