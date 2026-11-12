# Roadmap: F2 Algebra — CUDA Layout Symbolic Analysis

## Overview

Single-phase project: take the user's mid-development Go module from "does not compile" to a clean-building, correct F₂ algebra core. The user drives all further work — when they need help (bug hunts, tests, utilities), they call Claude in ad hoc with `/gsd:quick`. No multi-phase roadmap; this phase is the only planned work.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Build Fix & Working Algebra Core** - Module compiles and vets clean; expression tree and folding are correct

## Phase Details

### Phase 1: Build Fix & Working Algebra Core
**Goal**: The whole module builds and vets clean, and the F₂ expression tree with folding behaves correctly on hand-checked cases.
**Depends on**: Nothing (only phase)
**Requirements**: ALG-05, ALG-01, ALG-02
**Success Criteria** (what must be TRUE):
  1. `go build ./...` and `go vet ./...` pass with zero errors on the whole module — every package (`f2`, `check`, `common`) compiles, including syntax-error sites currently known in `expr.go` and `coeff_mat.go` and any latent errors in `vector.go`, `matop.go`, `visit.go`, `swizzle_map.go`, `access_map.go`, `stride_map.go`
  2. User can construct expressions (Variable / Const / Symbol / Xor / And) and compare them with structural equality
  3. Folding reduces constants and redundant operations (e.g., x⊕0→x, x⊕x→0, x·0→0, x·x→x) while preserving the tree-never-DAG invariant (no expression elimination)
  4. A smoke run builds an expression, folds it, and inspects the result without crashing or mis-printing
**Plans**: 2 plans

Plans:
- [ ] 01-01: Fix all syntax/build errors across internal/ until `go build ./...` passes
- [ ] 01-02: Fix core algebra bugs (fold semantics, structural equality, invariant preservation) until `go vet ./...` passes and fold behaves correctly on a hand-checked set of cases

### Unplanned (on-demand only)

The remaining v1 requirements are deliberately NOT planned into phases. The user will drive this work and call Claude in with `/gsd:quick` when needed:

- ALG-03, ALG-04 (vectors, coefficient matrices), TEST-01 — complete + test the vector/matrix layer
- MAP-01, MAP-02, MAP-03, TEST-02 — layout maps (access, swizzle, stride) + tests
- UTIL-01..04 — dumper, printer, equivalence checker, codegen

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Build Fix & Working Algebra Core | 0/2 | Not started | - |
