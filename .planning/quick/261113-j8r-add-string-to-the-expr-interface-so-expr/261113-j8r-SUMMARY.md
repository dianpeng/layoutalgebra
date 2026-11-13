---
phase: 261113-j8r
plan: 01
subsystem: core-algebra
tags: [go, interface, stringer, f2-algebra, dump]

# Dependency graph
requires:
  - phase: 261113-ir0
    provides: dump utilities with locked exact-string infix rendering (UTIL-01, UTIL-02)
provides:
  - Expr interface carrying String() string — any Expr-typed value prints via fmt directly
  - Four compile-time interface assertions (NodeVariable, NodeConst, NodeSymbol, NodeBinary)
  - Dispatcher-free dump.go; all rendering dispatches through Expr.String()
affects: [any phase rendering or diffing expressions, vectors, or coefficient matrices]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Interface-owned canonical render: String() lives on the Expr interface, not on a helper dispatcher"
    - "Compile-time assertions (var _ Expr = (*NodeX)(nil)) guard the interface contract"

key-files:
  created: []
  modified:
    - internal/f2/expr.go
    - internal/f2/dump.go
    - internal/f2/dump_test.go

key-decisions:
  - "New test calls tree.String() on an Expr-typed value so the pre-change compile failure IS the RED state, per the plan's TDD note; a pure fmt.Sprintf assertion would have passed pre-change via reflection-based Stringer detection"

patterns-established:
  - "Every render path (DumpPrefix, Vector.Dump, CoeffMat.Dump, NodeBinary.String) calls String() directly on the interface value"

requirements-completed: [UTIL-02]

# Metrics
duration: 5min
completed: 2026-11-13
---

# Quick 261113-j8r: Expr interface String() promotion Summary

**String() promoted into the Expr interface with four compile-time assertions; the unexported dumpExpr type-switch dispatcher deleted and its four call sites rerouted to direct interface calls — all locked exact-string dump tests pass unchanged plus a new interface-native printing test.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-11-13T05:49:00Z
- **Completed:** 2026-11-13T05:54:20Z
- **Tasks:** 1
- **Files modified:** 3

## Accomplishments
- `Expr` interface now declares `String() string`; `fmt.Sprintf("%v", expr)` on any Expr-typed value produces the locked fully-parenthesized infix form with no helper
- `var _ Expr = (*NodeVariable/NodeConst/NodeSymbol/NodeBinary)(nil)` assertions lock the contract at compile time
- `dumpExpr` dispatcher removed entirely; DumpPrefix, Vector.Dump, CoeffMat.Dump, and NodeBinary.String call `e.String()` on the interface value directly
- All 7 pre-existing exact-string dump tests pass unchanged; new `TestDumpExprInterfaceNative` covers the interface-native path

## Task Commits

TDD cycle, committed atomically per the plan's ordering:

1. **RED — failing test** - `1de6c1c` (test: TestDumpExprInterfaceNative; compile failure `tree.String undefined (type Expr has no field or method String)` was the red state)
2. **GREEN — interface change + dispatcher removal** - `ca5bc4b` (feat: String() on Expr, assertions, dumpExpr deleted, four call sites rerouted)

## Files Created/Modified
- `internal/f2/expr.go` - `String() string` added to Expr interface after Kind() with render-form comment; four compile-time assertions after NodeSymbol
- `internal/f2/dump.go` - dumpExpr function and doc comment deleted; header comment item (1) updated; four call sites now call `.String()` directly
- `internal/f2/dump_test.go` - `fmt` import added; `TestDumpExprInterfaceNative` asserts interface-native rendering of `((a * b) ^ c)`

## Decisions Made
- RED state realized as a compile failure (calling `tree.String()` on an Expr-typed value) rather than a runtime test failure — a pure `fmt.Sprintf` assertion would have passed pre-change because fmt detects Stringer implementations via reflection, which would have violated the fail-fast TDD gate
- Assertions placed after the NodeSymbol struct definition, covering all four node types in one contiguous block

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## TDD Gate Compliance

- RED gate: `test(261113-j8r)` commit `1de6c1c` — confirmed failing (compile error: `tree.String undefined`)
- GREEN gate: `feat(261113-j8r)` commit `ca5bc4b` — `go build ./... && go vet ./... && go test ./... -count=1` all green
- REFACTOR gate: not needed; no cleanup beyond the GREEN commit

## Self-Check

- `internal/f2/expr.go` contains `String() string` in the interface — FOUND (grep count 1)
- Four `var _ Expr` assertions at lines 94-97 — FOUND
- `grep -rn "dumpExpr" internal/f2/` returns nothing — CONFIRMED (exit 1)
- Commits `1de6c1c` and `ca5bc4b` exist on branch — CONFIRMED via git log

## Self-Check: PASSED

## Next Phase Readiness
- Rendering contract is now interface-native; any future Expr implementer gets compile-time enforcement of String()
- No blockers

---
*Phase: 261113-j8r*
*Completed: 2026-11-13*
