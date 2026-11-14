# F2 Algebra — CUDA Layout Symbolic Analysis

A Go library for **symbolic analysis of GPU memory layouts in F₂ algebra**.
It derives, verifies, and optimizes the bit-level mapping from **hardware
coordinates** (threadIdx-style lane/warp bits) to **tile memory offsets**,
producing an *offset vector* — one F₂ expression per offset bit — that a code
generator interprets directly into index arithmetic.

Think polyhedral layout optimization, but at **bit granularity** and over the
field with two elements: XOR is addition, AND is multiplication, and every
layout question becomes exact linear algebra.

```
hardware coordinate bits ──► [ layout maps ] ──► offset vector ──► codegen
(lane/warp variables,                            (one expression      (index
 symbolic M, N)                                   per offset bit)      arithmetic)
```

## Why F₂

CUDA/GPU layout decisions are bit decisions: which lane bit lands in which
offset bit decides SMEM bank conflicts, GMEM coalescing, and swizzle shape.
Restricted to power-of-2 tiles and strides (the model this library targets),
all of it is **linear over F₂** — permutations are permutation matrices,
XOR-swizzles are involutions, strides are shifts, and questions like
"is this mapping bijective?" or "how many distinct 32B sectors does this warp
touch?" become **exact rank computations** with provable answers, not
heuristics.

## Quick start

### GMEM coalescing optimization

```go
// Hardware coordinates: 2 warp bits + 5 lane bits (MSB-first, trailing
// lane bits = threadIdx.x convention).
hw := f2.NewVector([]f2.Expr{
    f2.NewVariable("w1"), f2.NewVariable("w0"),
    f2.NewVariable("t4"), f2.NewVariable("t3"), f2.NewVariable("t2"),
    f2.NewVariable("t1"), f2.NewVariable("t0"),
})

// Logical tile space: [i:2, j:5], float (dtypeSize 2).
res, err := f2.GmemOptimize(hw, []f2.GmemAxis{
    {Name: "i", Bits: 2}, {Name: "j", Bits: 5},
}, 5, 2)
// res.Offset — final offset-space bit pattern for code generation
// res.Plan   — layout report (innermost axis, fit/spill, matrix M)
// res.Score  — certified {LineRank, SectorRank}: one 128B line, four 32B
//              sectors = perfect coalescing (both volume floors met)
```

### SMEM bank-conflict optimization

```go
res, err := f2.SmemOptimize(hw, dataCoord, 2 /* float */, f2.Bank{
    Skip: 2, Range: 5, VariablePrefix: "t",   // CUDA banks: bits [2,7)
})
// res.Offset   — final offset-space bit pattern for code generation
// res.Swizzled — false = bank already saturated (conflict-free, unchanged)
```

Both pipelines emit the **same kind of artifact** — the offset vector — so
**one code generator serves both** memory hierarchies.

### Manual composition

Every pipeline stage is also a public building block:

```go
mapped, _  := f2.MapAccess(hw, dataCoord)              // bijective permutation
strided, _ := f2.StrideMap(shape, mapped, 2)           // byte-space shift
swizzled, _ := f2.SwizzleMap(bank, strided)            // XOR bank swizzle

cm, _ := swizzled.CoeffMatAll()                        // linearize: A·x
rank, _ := f2.RankConstMat(cm)                         // certify bijectivity
fmt.Print(swizzled.Dump())                             // "bit 0: ...\n..."
```

### Working with expressions directly

```go
x, y := f2.NewVariable("x"), f2.NewVariable("y")
m := f2.NewSymbol("M")                                   // static parameter

e := f2.NewBinary(f2.Xor, x, f2.NewBinary(f2.Mul, m, x)) // x + M·x
fmt.Println(f2.Fold(e))                                  // x * (1 ^ M) — canonical
concrete := f2.Substitute(e, map[string]f2.Expr{"M": f2.NewConstOne()})
```

## The layout model

The optimizer integration is three steps, mirroring how a GPU kernel
actually addresses memory:

1. **Hardware coordinates** — the user provides the coordinate vector:
   lane bits, warp bits, block bits, named and ordered MSB-first.
2. **Layout maps** — a chain of F₂-linear transformations from hardware
   coordinates to the final offset space:
   - *Access / coalescing map* `M`: hardware → logical data coordinates
     (bijective permutation — `MapAccess`, `GmemCoalescingMap`)
   - *Stride map*: logical coordinates → byte space (`StrideMap`)
   - *Swizzle map* (SMEM only): XOR bank-conflict elimination
     (`SwizzleMap` — a bijective involution: applying it twice restores
     the input)
3. **Code generation** — interpret the final offset vector into emitted
   index arithmetic. Each bit's expression is already in canonical form
   (fold-complete, order-sorted), so equal subexpressions print and emit
   identically — CSE comes free.

## Mathematical guarantees

These are engineered and **machine-verified**, not aspirational
(`internal/f2/theory.md` has the full model):

- **Canonical forms.** The fold engine rewrites every expression to a
  unique normal form via six rules over a structural total order
  (`Const < Symbol < Variable < Binary`, then value/name/children
  lexicographically). Verified by enumerating **all 10,788 expressions up to
  4 leaves** and asserting the canonical-form predicate + idempotence.
- **Convergence = termination + confluence.** Every rule application
  strictly decreases its own well-founded witness (checked per application
  on the enumeration), and folding under randomly permuted rule orders
  yields structurally identical results (Newman's condition).
- **Linearity invariants.** Every mapping is bijective by construction
  (permutation), or injective with the guard contract (stride class);
  certification is exact F₂ rank via Gaussian elimination.
- **Determinism.** The whole suite passes under `-count=20`.

Rule of the framework: **conservative** — do what it can, return an `error`
when it cannot or does not know what it is doing. False negatives
(bailouts) are acceptable; false positives (wrong math) and silent failures
are not.

## Conventions

| Convention | Detail |
|---|---|
| Vector order | **MSB-first**: index 0 is the most significant bit (`ShiftMatrix` semantics). The lowest byte weights sit at the *end* of a vector. |
| Lane bits | The **trailing** entries of the hardware vector are the warp lane bits (CUDA linear-thread-index convention: `threadIdx.x` is fastest-varying). |
| Bank / segment windows | `Bank{Skip, Range}` = bits `[Skip, Skip+Range)`; CUDA SMEM banks are `{2, 5}`, GMEM sectors are `{5, ∞)`, lines `{7, ∞)`. |
| Expressions | Trees, never DAGs — simplification never eliminates expressions (project invariant). |
| Scalars | `type F2 bool` (defined type — Go forbids methods on aliases); `Zero`/`One` constants. |
| Static parameters | Symbols (`M`, `N`) — substituted with concrete values at code-generation time via `Substitute`. |

## Verification

| Gate | What it proves | Where |
|---|---|---|
| Canonical enumeration | all depth-≤4 expressions fold to canonical form, idempotently | `canon_test.go` |
| Termination witnesses | every rule application decreases its witness | `convergence_test.go` |
| Confluence | permuted rule orders → identical canonical forms | `convergence_test.go` |
| CUDA reference scenarios | warp access (saturated bank), strided access (swizzle repair 3→5), full-pipeline runs | `layout_cuda_test.go`, `pipeline_test.go` |
| Full suite | 71 test functions, `go vet` clean, `gofmt` clean, `-count=20` stable | — |

Run it:

```bash
go build ./... && go vet ./... && go test ./... -count=1
./scripts/format.sh   # keep gofmt-clean
```

## Project layout

```
internal/f2/
  expr.go                 expression trees, F₂ scalars, structural equality
  order.go                OTuple — the structural total order
  fold.go                 the rewrite engine (6 rules) + driver
  visit.go                tree visitors (pre/post/by-order)
  vector.go               bit vectors, coefficient extraction
  coeff_mat.go            coefficient matrices, matrix-vector product
  matop.go                F₂ Gaussian elimination, rank, permutation/shift matrices
  access_map.go           hardware → data coordinate permutation
  stride_map.go           coordinate → byte space
  swizzle_map.go          SMEM bank-conflict XOR swizzle (bijective involution)
  gmem_coalescing_map.go  substitution, coalescing score, the M generator
  pipeline.go             GmemOptimize / SmemOptimize — the two entry points
  dump.go                 String()/DumpPrefix/vector/matrix rendering
  theory.md               the formal model: lattice, witnesses, confluence
```

## Status

Complete and verified: core F₂ algebra (expressions, fold engine), vectors
and coefficient matrices, exact elimination, all four layout map types,
both optimization pipelines, and the dump utilities.

Deliberately out of scope (the F₂ model does not cover them): affine/constant
offsets (handled by the emitter as integer adds on top of the linear part),
non-power-of-2 pitches (multiplication by non-power-of-2 constants involves
carries — outside F₂-linearity), and symbolic-coefficient rank (bijective
cores are constant-coefficient by design; symbols live in the stride class).

Next per the requirements ledger: `UTIL-03` (equivalence checker) and
`UTIL-04` (code generation — the offset-vector interpreter), plus the
extraction hardening sweep (remaining assertion sites converted to errors).

## Development model

Authored by [dianpeng](https://github.com/dianpeng); Claude assists with
bug fixes, tests, and utilities on request. Conventions: conventional
commits, stdlib-only tests (table-driven, exact assertions), zero
third-party dependencies, and every fix ships with a test that fails
without it. Planning artifacts live in `.planning/` (GSD workflow).
