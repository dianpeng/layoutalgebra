---
type: quick
date: 2026-11-17
---

# Rename project to Layout Algebra + universal interface scaffolding

## Description

User-directed refactor: (1) project is no longer f2-only, rename module to
`github.com/dianpeng/layoutalgebra`; (2) the repo provides multiple layout-algebra
implementations for upper-layer integration behind a universal interface; (3) per-algebra
AST/codegen comes later (not now) and will plug in behind the interface; (4) all imports
moved off `f2algebra`.

## Tasks (all done)

1. go.mod + all .go imports: `github.com/dianpeng/f2algebra` -> `github.com/dianpeng/layoutalgebra`;
   stray doc comment in internal/f2/vector.go fixed.
2. Root package `layoutalgebra`: doc.go (3-layer architecture: universal interface /
   algebra implementations / future codegen backends), algebra.go — `Problem` (Tile,
   Concurrency variables, Lane) with Validate, opaque `Mapping`/`Offset`, the `Algebra`
   interface (Name/Supported/Access/StrideMap/GmemOptimize/SmemOptimize), and a
   name registry (Register/Lookup/Names). Deliberately no f2/subgroupchain adapters yet —
   each algebra's adapter lands with its own work (subgroupchain GMEM/SMEM is
     user-implemented right now).
3. algebra_test.go: Problem.Validate table test, registry tests incl. duplicate/nil panic.
4. Root README: umbrella title/intro, Architecture section, project-layout + status update.
   CLAUDE.md project block updated (multi-algebra description, module path).

## Verification

- `go build ./...`, `go vet ./...` clean; `go test ./... -count=1` all packages pass.

## Notes

- The on-disk directory is still named `f2algebra/`; only the Go module path matters to
  the toolchain. User may `mv` the directory at leisure.
