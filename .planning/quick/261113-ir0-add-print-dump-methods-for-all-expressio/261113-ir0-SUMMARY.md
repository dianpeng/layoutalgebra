---
phase: 261113-ir0
plan: 01
subsystem: internal/f2
tags: [f2-algebra, utilities, dump, printing]
requires: []
provides:
  - "fmt.Stringer on F2/NodeVariable/NodeConst/NodeSymbol/NodeBinary (infix, fully parenthesized)"
  - "DumpPrefix(Expr) operator-first prefix render"
  - "Vector.Dump / CoeffMat.Dump / ConstMat.Dump per-bit/per-row line renders"
affects: [internal/f2/dump.go, internal/f2/dump_test.go]
tech-stack:
  added: []
  patterns: ["strings.Builder accumulation", "unexported dumpExpr type-switch dispatcher"]
key-files:
  created: [internal/f2/dump.go, internal/f2/dump_test.go]
  modified: []
decisions:
  - "dumpExpr(e Expr) unexported type-switch helper dispatches String() because the Expr interface does not carry String (auto-fix, Rule 3)"
  - "ConstMat.Dump reads Raw() [][]F2 directly to avoid per-element NodeConst allocation (plan-sanctioned)"
metrics:
  duration: ~6 min
  completed: 2026-11-13
  tasks: 2
  files: 2
---

# Quick Task 261113-ir0: Print/Dump Methods for All Expression Types Summary

Infix (`fmt.Stringer`), prefix (`DumpPrefix`), and per-stage (`Vector.Dump`,
`CoeffMat.Dump`, `ConstMat.Dump`) renderers added in a single new file
`internal/f2/dump.go`, locked by exact-string table-driven tests in
`internal/f2/dump_test.go` — all stdlib-only, all read-only, zero edits to
existing user files.

## What Was Built

- **Task 1** (`internal/f2/dump.go`, commit `c76318e`):
  - `F2.String()` -> "0"/"1"; `NodeVariable`/`NodeSymbol` -> name verbatim;
    `NodeConst` -> "0"/"1"; `NodeBinary` -> fully parenthesized infix
    (`(a ^ (b * c))`, `^` for Xor/Add, `*` for Mul/And) via `dumpOpSym`.
  - `DumpPrefix(e Expr)` -> operator-first fully parenthesized (`(^ a (* b c))`).
  - `Vector.Dump()` -> one `bit i: <expr>` line per index, trailing newline.
  - `CoeffMat.Dump()` / `ConstMat.Dump()` -> m rows of `[elem elem ...]`, trailing
    newline; ConstMat reads `Raw()` directly.
- **Task 2** (`internal/f2/dump_test.go`, commit `5a9480f`): seven tests —
  `TestDumpNodeString`, `TestDumpPrefix`, `TestDumpVector`, `TestDumpCoeffMat`,
  `TestDumpConstMat`, `TestDumpPipelineStageEndToEnd` (locks the exact 4-line
  `MapAccess -> StrideMap -> SwizzleMap` dump `"bit 0: l_0\nbit 1: (l_0 ^ l_1)\nbit 2: l_1\nbit 3: l_2\n"`
  and the 2-line `CoeffMatAll` dump `"[1 0]\n[1 1]\n"`), and `TestDumpReadOnly`
  (renders leave all node markers at 0 and preserve `IsEqual` against a fresh tree).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `Expr` interface has no `String()` method**
- **Found during:** Task 1 (compile failed: `self.L().String undefined (type Expr has no field or method String)`)
- **Issue:** The plan's locked snippets call `.String()` on `Expr`-typed values
  (`self.L()`, `Index(i)`, `DumpPrefix` leaf case), but `Expr` only declares
  `Kind/IsEqual/SetMarker/Marker`.
- **Fix:** Added unexported `dumpExpr(e Expr) string` — a type switch over the
  four concrete node kinds dispatching to their `String()`. All render sites
  (`NodeBinary.String`, `DumpPrefix`, `Vector.Dump`, `CoeffMat.Dump`) route
  through it. All plan grep gates still pass (5 String + 1 DumpPrefix + 3 Dump),
  format decisions unchanged.
- **Files modified:** internal/f2/dump.go
- **Commit:** c76318e

**2. [Rule 1 - Bug] Vet failure on non-addressable `CoeffMatAll` receiver**
- **Found during:** Task 2 (`go vet`: cannot call pointer method `CoeffMatAll` on `Vector` returned by `NewVector`)
- **Fix:** Assigned the vector to a local variable before calling `CoeffMatAll()` in
  `TestDumpPipelineStageEndToEnd`. Test-only, no behavior change.
- **Files modified:** internal/f2/dump_test.go
- **Commit:** 5a9480f

No other deviations — every locked expected-output string passed empirically on
first run; none were edited.

## Verification Results

- Baseline (pre-change): `go build ./... && go vet ./... && go test ./internal/f2/` — green.
- Task 1 after-checkpoint: build + vet + f2 tests green; grep gates: 5 `String()`,
  1 `DumpPrefix`, 3 `Dump` definitions — PASS.
- Task 2 after-checkpoint: `go vet ./...` clean; `go test ./internal/f2/ -run 'Dump' -v`
  — 7/7 PASS; `go test ./...` fully green; zero `"github.com` imports in
  dump_test.go — PASS.

## Known Stubs

None — all renders are wired to real data (expression trees, vectors, matrices)
with exact-string assertions.

## Threat Flags

None — no new trust boundaries, I/O, or network surface (matches plan threat
model: host-side pure library code; read-only rendering locked by
`TestDumpReadOnly`).

## Self-Check

- FOUND: internal/f2/dump.go (commit c76318e)
- FOUND: internal/f2/dump_test.go (commit 5a9480f)
- FOUND: commits c76318e, 5a9480f in `git log`
- Zero edits to existing source files: confirmed (`git show --stat` on both commits)

## Self-Check: PASSED
