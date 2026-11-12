# Requirements: F2 Algebra — CUDA Layout Symbolic Analysis

**Defined:** 2026-11-12
**Core Value:** Given hardware coordinate bits, correctly and verifiably derive the symbolic bit-level mapping to a tile offset as an F₂ vector the user can interpret into generated code.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Core Algebra

- [ ] **ALG-01**: F₂ expression tree (Variable / Const / Symbol / Binary over Xor·And) with structural equality and the tree-never-DAG invariant (no expression elimination during simplification)
- [ ] **ALG-02**: Expression folding/simplification that preserves input tree structure while reducing constants and redundant operations
- [ ] **ALG-03**: F₂ vector representation — one expression per output bit over input coordinate bits, with vector-level ops
- [ ] **ALG-04**: Coefficient matrices and matrix operations (multiply, add, transpose/reduce) over F₂ expressions
- [x] **ALG-05**: `go build ./...` and `go vet ./...` pass clean on the whole module

### Layout Maps

- [ ] **MAP-01**: Access map — derive symbolic mapping from threadIdx coordinate bits to tile (M, N) offset via the F₂ interpreter
- [ ] **MAP-02**: Swizzle map construction and composition with access maps
- [ ] **MAP-03**: Stride map construction and composition with access maps

### Testing

- [ ] **TEST-01**: Table-driven stdlib unit tests covering internal/f2 (expressions, fold, vector, coefficient matrices)
- [ ] **TEST-02**: Table-driven stdlib unit tests covering layout map types (access_map, swizzle_map, stride_map)

### Utilities

- [ ] **UTIL-01**: Expression dumper — render expression trees as infix/prefix strings with variable names
- [ ] **UTIL-02**: Vector/matrix pretty printer — formatted per-bit F₂ expression tables
- [ ] **UTIL-03**: Simplifier + equivalence checker for comparing layout mappings
- [ ] **UTIL-04**: Code generation — interpret the final F₂ vector into emitted index-arithmetic code

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Broader Symbolism

- **SYMB-01**: Fully symbolic layout parameters (swizzle bits, strides) in the algebra, beyond symbolic M, N

### Validation

- **VALD-01**: Concrete reference validation against known-good layouts (e.g., CUTLASS swizzle patterns) for concrete tile shapes

## Out of Scope

| Feature | Reason |
|---------|--------|
| External test dependencies (testify, property-based libs) | User decision: stdlib-only table-driven tests |
| blockIdx / multi-tile composition | Keep algebra domain minimal (threadIdx → tile offset) until core is validated |
| CUDA kernel execution / GPU-side validation | Host-side symbolic tool only |
| Rewriting user-authored core code wholesale | Passive-assistance model: user writes core, Claude fixes bugs/tests/utilities |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| ALG-01 | Phase 1 | Pending |
| ALG-02 | Phase 1 | Pending |
| ALG-03 | — (on-demand via /gsd:quick) | Pending |
| ALG-04 | — (on-demand via /gsd:quick) | Pending |
| ALG-05 | Phase 1 | Complete |
| MAP-01 | — (on-demand via /gsd:quick) | Pending |
| MAP-02 | — (on-demand via /gsd:quick) | Pending |
| MAP-03 | — (on-demand via /gsd:quick) | Pending |
| TEST-01 | — (on-demand via /gsd:quick) | Pending |
| TEST-02 | — (on-demand via /gsd:quick) | Pending |
| UTIL-01 | — (on-demand via /gsd:quick) | Pending |
| UTIL-02 | — (on-demand via /gsd:quick) | Pending |
| UTIL-03 | — (on-demand via /gsd:quick) | Pending |
| UTIL-04 | — (on-demand via /gsd:quick) | Pending |

**Coverage:**
- v1 requirements: 14 total
- Mapped to phases: 3 (single-phase roadmap per user decision — user drives the rest via /gsd:quick)
- Unmapped: 11 (deliberately on-demand, not a gap)

---
*Requirements defined: 2026-11-12*
*Last updated: 2026-11-12 after roadmap creation*
