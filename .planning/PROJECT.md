# F2 Algebra — CUDA Layout Symbolic Analysis

## What This Is

A Go library for symbolic analysis of CUDA layout calculation, written as a basic F₂ algebra.
The user sets up an F₂ algebra interpreter parametrized by symbolic tile dimensions (M, N); the
library performs symbolic calculation to generate a mapping from **hardware coordinates**
(threadIdx.x/y/z bits) to a **final memory offset** in a tile. The result is a *vector* — one
F₂ expression per output offset bit, over the input coordinate bits. From that vector the user
generates code directly by interpreting it.

Claude's role here is **passive assistance**: the user writes the code and calls Claude in for
(1) finding bugs and logical issues, (2) completing code — fixing bugs and writing comprehensive
unit tests, (3) utility features (expression dumpers, matrix/vector printers, etc.).

## Core Value

Given hardware coordinate bits, correctly and verifiably derive the symbolic bit-level mapping
to a tile offset as an F₂ vector the user can interpret into generated code.

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

(None yet — ship to validate)

### Active

<!-- Current scope. Building toward these. -->

- [ ] Core F₂ expression tree (variables, constants, binary XOR/AND ops) with structural equality — drafted in `internal/f2/expr.go`
- [ ] Expression simplification/folding (`internal/f2/fold.go`) — drafted
- [ ] F₂ vector representation over expression trees (`internal/f2/vector.go`) — drafted
- [ ] Coefficient matrix + matrix ops (`coeff_mat.go`, `matop.go`) — drafted
- [ ] Layout map types: access map, swizzle map, stride map — drafted
- [ ] Expression visitor (`internal/f2/visit.go`) — drafted
- [ ] Tree compiles cleanly (`go build ./...` passes); currently does not — syntax errors across `expr.go`, `coeff_mat.go`
- [ ] Comprehensive unit tests (stdlib `testing`, table-driven) for all packages
- [ ] Utility: expression dumper (infix/prefix string output with variable names)
- [ ] Utility: vector/matrix pretty printer (per-bit F₂ expression tables)
- [ ] Utility: simplifier + equivalence checker
- [ ] Utility: code generation — interpret the final vector into emitted index-arithmetic code

### Out of Scope

<!-- Explicit boundaries. Includes reasoning to prevent re-adding. -->

- External test dependencies (testify, property-based libs) — user wants stdlib-only table-driven tests
- Mapping anything other than threadIdx-style hardware coordinates → tile offset (no blockIdx, no multi-tile composition) — keep the algebra domain minimal until core is validated
- Full symbolic dimensions beyond M, N (swizzle bits, strides stay concrete at analysis time)
- CUDA kernel execution / GPU-side validation — this is a host-side symbolic tool

## Context

- Language: Go (toolchain go1.26.2), module `github.com/dianpeng/f2algebra`, packages under `internal/` (`f2`, `check`, `common`).
- Existing code (~2000 lines) is user-written and mid-development: core algebra mostly written, edges rough, **tree does not compile yet** (known syntax errors in `expr.go`, `coeff_mat.go`; import typo in `vector.go` already fixed, `common.IsPowerOf2` parameter bug already fixed).
- Design invariant (from `expr.go` doc comment): expressions are always trees, never DAGs — simplification never performs expression elimination; each node is unique in the tree. This guarantees correctness/simplicity and must be preserved.
- Expression ops: `Xor` (Add), `And` (Mul) over `F2 = bool` scalars; `NodeKind` = Variable | Const | Binary | Symbol.
- Correctness verification approach: TBD per case when the user calls Claude in — options include expr-level equivalence checks and concrete reference checks against known layouts.
- Workflow model: user drives; Claude is on-call for bug hunts, test-writing, and utilities. Planning docs should respect that — phases are shaped around what the user asks for next, not autonomous feature expansion.

## Constraints

- **Tech stack**: Go stdlib only for tests — no external test frameworks or assertion libraries.
- **Dependencies**: keep third-party imports near zero; this is a small self-contained algebra library.
- **Authorship model**: user writes the core code; Claude fixes bugs, writes tests, and adds utilities on request — avoid rewriting user code wholesale.
- **Correctness**: F₂ bit-level math is unforgiving; every bug fix and utility must come with unit tests.

## Key Decisions

<!-- Decisions that constrain future work. Add throughout project lifecycle. -->

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Module path `github.com/dianpeng/f2algebra` | Matches existing import paths in the source tree | — Pending |
| Expressions stay trees, never DAGs; no expression elimination in simplification | User's core design invariant for correctness guarantees | — Pending |
| Symbolic M, N as compile-time parameters; concrete substitution at code-gen time | User's answer on parametrization: vector is generic, codegen substitutes values | — Pending |
| Verification approach decided case-by-case | User prefers to decide as things come up | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-11-12 after initialization*
