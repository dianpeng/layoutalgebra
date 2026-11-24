# Layout Algebra — CUDA Layout Symbolic Analysis

A Go library of **symbolic layout algebras for GPU memory analysis**, unified
behind a single interface. Each algebra derives, verifies, and optimizes the
mapping from **hardware concurrency** (threadIdx-style lane/warp variables)
to **tile memory offsets**, producing a symbolic artifact that a code
generator interprets directly into index arithmetic.

Implemented today:

- **F₂ algebra** (`internal/f2`) — bit-level analysis over the field with two
  elements: XOR is addition, AND is multiplication, every layout question is
  exact linear algebra. Power-of-2 tiles and strides. See below.
- **Z/k sub-group chain algebra** (`internal/subgroupchain`) — CuTE-style
  hierarchical tuples (HTuple) over arbitrary tile factorizations, with
  bag-of-variables (BoV) access mapping and stride mapping. See
  `internal/subgroupchain/README.md`.

```
hardware concurrency ──► [ algebra ] ──► universal AST ──► compiler backend
(lane/warp variables,     (f2 /          (pkg/ast via        (consumes pkg/ast +
 tile shape, dtype)        subgroupchain)  pkg/layout)         pkg/layout only)
```

## Architecture

The module is three layers:

1. **The universal interface** (package `layoutalgebra`, repo root) —
   `Algebra` + `Problem`/`Mapping`/`Offset` + a name registry. Upper layers
   integrate against this interface only; they pick an implementation by name
   or capability and drive the pipeline stages (`Access`, `StrideMap`,
   `GmemOptimize`, `SmemOptimize`) without knowing the algebra underneath.
2. **The algebra implementations** (`internal/f2`, `internal/subgroupchain`)
   — each self-contained, with its own native API beside the universal one,
   its own guarantees, and its own test suite.
3. **The universal integration layer** (`pkg/ast`, `pkg/layout`) — the sealed
   algebra-agnostic AST plus the abstract-factory facade and the root-registry
   adapters. A compiler backend integrates against `pkg/ast` +
   `pkg/layout` only (see *Universal integration* below); the per-algebra
   backends that emit index arithmetic from the AST are the remaining piece.

Conventions for implementations: conservative contract (do what you can,
`error` when you cannot — false negatives are acceptable, wrong math and
silent failures are not); implementations never mutate the `Problem`; results
are opaque outside the producing algebra.

## Universal integration (pkg/ast + pkg/layout)

Layer 3 for *consumers*: `pkg/layout` is the abstract factory (pick an algebra
by `Kind`, get stage artifacts as algebra-agnostic `ast.Expr` trees) and
`pkg/ast` is the one sealed interchange AST a compiler translates into its own
IR — no `internal/` types ever cross this boundary. Importing `pkg/layout`
also registers both algebras in the root `layoutalgebra` registry.

Consumers import the root package (for `Problem`/`Tile`/`Variable`) plus the
two integration packages:

```go
import (
    "github.com/dianpeng/layoutalgebra"
    "github.com/dianpeng/layoutalgebra/pkg/ast"
    "github.com/dianpeng/layoutalgebra/pkg/layout"
)
```

The native algebra APIs live under `internal/` and are in-module only, so
out-of-module consumers integrate against `pkg/layout` + `pkg/ast` alone:

```go
eng, _ := layout.New(layout.SubgroupChain) // or layout.F2 for power-of-2 tiles
sess, _ := eng.NewSession(layoutalgebra.Problem{
    Tile:        layoutalgebra.Tile{Shape: []int{7, 16, 4}, DTypeSize: 4},
    Concurrency: []layoutalgebra.Variable{
        {Family: "warp", Name: "w", Bound: 14},
        {Family: "lane", Name: "t", Bound: 32}, // lane last: sgc BoV order
    },
    Lane: "t",
})
base, _ := sess.Access()        // base.Access (Tuple), base.Offset
gmem, _ := sess.GmemOptimize()  // gmem.Offset -> translate into compiler IR
smem, _ := sess.SmemOptimize()
off, _ := ast.Eval(gmem.Offset, map[string]int{"w": 3, "t": 17}) // reference evaluation
_ = off
```

Choosing F₂ vs sub-group chain is a one-token change (`layout.F2` ↔
`layout.SubgroupChain`): identical stage methods, identical artifact types —
provided the problem satisfies the chosen algebra's preconditions (F₂ wants
power-of-2 tile extents; the sub-group chain wants Π(bounds) == Π(extents)).
Both are checked loudly at `NewSession`.

| Option                | Default | Feeds                                                        |
| --------------------- | ------- | ------------------------------------------------------------ |
| `WithLineBytes(n)`    | 128     | F₂ `Segment{Sector:5, Line:log2(n)}`, sgc `GMemCoalesce.ByteSize` |
| `WithBankBytes(n)`    | 128     | sgc `SmemCoalesce.BankBytes`                                  |
| `WithBankWindow(s,r)` | (2, 5)  | F₂ `Bank{Skip, Range}`                                         |

Stage artifacts (`layout.Mapping`): `Access` is the tile-coordinate mapping
AST (F₂: one `Tuple` child per bit, MSB-first; sub-group chain: a hierarchy
mirroring the tile shape) and `Offset` is the final byte-offset AST. For the
F₂ SMEM stage, `Access` is the *pre-swizzle* coordinate mapping — the swizzle
lives in byte space, so `Offset` alone carries it. Registry consumers can
also drive the root `layoutalgebra.Algebra` adapters directly and lower
afterwards with `layout.LowerAccess` / `layout.LowerOffset`.

The lowered ASTs are machine-checked against the native pipelines: for every
assignment of the concurrency variables on enumerated small problems,
`ast.Eval(offset)` equals the native pipeline's byte offset, for both
algebras and all three stages, and access/optimized offsets stay bijections
onto the tile's byte multiples.

### Runnable examples

```bash
go run ./examples/compilerpass    # consumer-side "pass" pattern, both algebras
go run ./examples/gmem-coalescing # focused GMEM coalescing (sub-group chain)
go run ./examples/smem-swizzle    # SMEM bank-conflict analysis (F₂)
```

See `examples/README.md` for what each demonstrates. Together they cover the
facade end to end: the one-token algebra switch (`layout.F2` ↔
`layout.SubgroupChain`) with identical stage calls and artifact types, all
three stage methods per algebra (`Access`, `GmemOptimize`, `SmemOptimize`)
with each `Offset` AST printed in infix form, an `SmemOptimize` conservative
error handled informationally alongside a saturated success, `ast.Eval`
reference evaluation on fixed assignments, `ast.Dump` tree rendering, and a
lane-first sub-group-chain problem where `GmemOptimize` actually rewrites
the mapping (draws lane freedom into the last dimension).

The two algebras' lowered ASTs consume differently-named `ast.Eval`
assignments: F₂ works at bit granularity, so its ASTs reference per-bit
variables — bit `j` of variable `v` is named `v`+`j` (`w0`, `t1`, ...) — and
a variable-level assignment like `{w: 2, t: 5}` must be expanded to bit
level first (see `expandBits` in `examples/compilerpass/main.go`);
sub-group-chain ASTs
consume the variable-level names (`w`, `t`) directly.

---

# The F₂ Algebra

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

## Pipeline API

### `GmemOptimize` — GMEM coalescing

```go
func GmemOptimize(hw Vector, axes []GmemAxis, laneBits, dtypeSize int) (*GmemResult, error)
func GmemOptimizeWith(hw Vector, axes []GmemAxis, laneBits, dtypeSize int, seg Segment) (*GmemResult, error)
```

Derives the bijective transformation `M` (hardware → data logical
coordinates) with the provably optimal greedy — lane bits into the lowest
logical positions — applies it, shifts into byte space, and certifies the
lane-restricted coalescing objective. Returns:

| Field | Meaning |
|---|---|
| `Offset` | final offset-space `Vector` for code generation |
| `Plan` | `*CoalescePlan`: `Matrix` (M), `Innermost` axis, `Span`, `LaneBitsFit`/`LaneBitsSpill`, `Perfect` |
| `Score` | `CoalesceScore{LineRank, SectorRank}` — the warp touches `2^LineRank` lines and `2^SectorRank` sectors; sectors are the transaction cost |

The score is certified against the **volume floors** `sectorRank ≥ laneBits
+ dtypeSize − Sector` and `lineRank ≥ max(0, laneBits + dtypeSize − Line)`;
both met = perfect coalescing. Segment boundaries default to
`GmemSegmentNvidia = {Sector: 5, Line: 7}` (32B sectors, 128B lines);
`GmemOptimizeWith` takes an explicit `Segment` for other architectures.

### `SmemOptimize` — SMEM bank-conflict elimination

```go
func SmemOptimize(hw Vector, dataCoord Vector, dtypeSize int, bank Bank) (*SmemResult, error)
```

Maps hardware coordinates to the logical data layout, shifts into byte
space, and applies the XOR swizzle. `dataCoord` selects the mode by shape:

- **Same size as `hw`** — routed through `MapAccess`: a permutation of the
  hardware coordinates (bijective by construction).
- **Different size** — used directly as the logical coordinate expressions
  over the hw basis: the rectangular/injective patterns (multi-thread per
  element, sub-tile access) that `MapAccess` cannot express, and the cases
  where swizzling actually has work to do.

Returns `Offset` (final vector for codegen) and `Swizzled`. Error
semantics follow the conservative contract: `SwizzleErrorSaturated` is a
**success** — the bank slice is already fully ranked (conflict-free) and the
strided layout is returned unchanged with `Swizzled=false`; other failures
(e.g. `SwizzleErrorNoFreeDegree`) propagate as errors.

### Manual composition

Every pipeline stage is also a public building block:

```go
mapped, _  := f2.MapAccess(hw, dataCoord)              // bijective permutation
strided, _ := f2.StrideMap(shape, mapped, 2)           // byte-space shift
swizzled, _ := f2.SwizzleMap(bank, strided)            // XOR bank swizzle

plan, _  := f2.GmemCoalescingMap(hw, axes, 5, 2)       // the coalescing M + report
score, _ := f2.ScoreCoalescingWith(offset, lanes, seg) // certified objective
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
   coordinates to the final offset space, composed by the two pipeline
   entry points:
   - *GMEM*: `GmemCoalescingMap` (M: hardware → logical data coordinates,
     bijective permutation) → `StrideMap` → certified `ScoreCoalescing`
   - *SMEM*: `MapAccess` (hardware → logical data coordinates) →
     `StrideMap` → `SwizzleMap` — the swizzle is a bijective involution:
     applying it twice restores the input
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
| Bank / segment windows | `Bank{Skip, Range}` = bits `[Skip, Skip+Range)`; CUDA SMEM banks are `{2, 5}`. GMEM boundaries are the `Segment{Sector, Line}` log2 sizes — NVIDIA default `GmemSegmentNvidia = {5, 7}` (32B sectors, 128B lines); other architectures declare their own. |
| Expressions | Trees, never DAGs — simplification never eliminates expressions (project invariant). |
| Scalars | `type F2 bool` (defined type — Go forbids methods on aliases); `Zero`/`One` constants. |
| Static parameters | Symbols (`M`, `N`) — substituted with concrete values at code-generation time via `Substitute`. |

## Verification

| Gate | What it proves | Where |
|---|---|---|
| Canonical enumeration | all depth-≤4 expressions fold to canonical form, idempotently | `canon_test.go` |
| Termination witnesses | every rule application decreases its witness | `convergence_test.go` |
| Confluence | permuted rule orders → identical canonical forms | `convergence_test.go` |
| CUDA reference scenarios | warp access (saturated bank), strided access (swizzle repair 3→5), full-pipeline runs, custom-segment scoring | `layout_cuda_test.go`, `pipeline_test.go`, `gmem_coalescing_map_test.go` |
| Full suite | 71 test functions, `go vet` clean, `gofmt` clean, `-count=20` stable | — |

Run it:

```bash
go build ./... && go vet ./... && go test ./... -count=1
go run ./examples/compilerpass   # end-to-end tour of the integration layer
./scripts/format.sh   # keep gofmt-clean
```

## Project layout

```
layoutalgebra (repo root)
  doc.go / algebra.go       the universal Algebra interface + registry (integration point)
pkg/ast/                    the universal compiler-interchange AST (11 sealed nodes)
  ast.go                  node set + constructors + Children/Walk
  eval.go                 Eval — the reference integer evaluator
  print.go                String() (infix) + Dump (multi-line prefix form)
pkg/layout/                 the layer-3 facade + root-registry adapters
  layout.go               Kind, options, Engine/Session/Mapping abstract factory
  f2.go                   F₂ backend + root-Algebra adapter (init-registered)
  subgroupchain.go        sub-group chain backend + root-Algebra adapter
  lower.go                per-algebra lowering + LowerAccess/LowerOffset dispatch
examples/                   runnable consumer programs (see examples/README.md)
  compilerpass/           the "pass" pattern: both algebras through the facade
  gmem-coalescing/        GMEM coalescing on the sub-group-chain backend
  smem-swizzle/           SMEM bank-conflict analysis on the F₂ backend
internal/f2/                the F₂ algebra (power-of-2 tiles)
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
internal/subgroupchain/     the Z/k sub-group chain algebra (arbitrary extents)
internal/common/            shared numeric helpers (GCD, strides, products)
internal/check/             invariant-assertion guards (Check/DCheck/Unreachable)
```

## Documentation

Theory and consumer guides for both algebras live under `doc/`:

- `doc/f2.md` — the F₂ algebra: GF(2) arithmetic, the bit-vector offset
  model, the guarantees the code enforces, and when to choose it.
- `doc/subgroupchain.md` — the Z/k sub-group chain algebra: hierarchical
  tuples, bag-of-variables access mapping, conservative optimization, and
  when to choose it.

Both end with a "Which algebra?" decision section. The internal
`internal/f2/theory.md` (fold-engine formal model) and
`internal/subgroupchain/README.md` (data-structure notes and pass design)
remain the drift-prone primary sources; the `doc/` files link back to them
rather than duplicating them.

## Status

The F₂ algebra is complete and verified: core F₂ algebra (expressions, fold
engine), vectors and coefficient matrices, exact elimination, all four layout
map types, both optimization pipelines, and the dump utilities.

The sub-group chain algebra provides its access and stride mappings with
machine-verified bijection soundness (see its README), plus its GMEM/SMEM
optimization passes.

The universal integration layer has landed: both algebras are registered in
the root `layoutalgebra` registry via `pkg/layout`'s adapters, and the
`pkg/ast` + `pkg/layout` facade gives consumers algebra-agnostic stage
artifacts. What remains deferred is the per-algebra *compiler backend* that
consumes the universal AST and emits index arithmetic. `go run
./examples/compilerpass` tours the landed facade end to end on both algebras.

Deliberately out of scope (the F₂ model does not cover them): affine/constant
offsets (handled by the emitter as integer adds on top of the linear part),
non-power-of-2 pitches (multiplication by non-power-of-2 constants involves
carries — outside F₂-linearity), and symbolic-coefficient rank (bijective
cores are constant-coefficient by design; symbols live in the stride class).
