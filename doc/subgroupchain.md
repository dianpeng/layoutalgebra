# The Z/k Sub-group Chain Algebra — Theory and Use

This document explains the mathematics behind the sub-group chain algebra
(`internal/subgroupchain`): Z/k sub-group chains, hierarchical tuples over
arbitrary tile factorizations, the bag-of-variables access model, and the
conservative optimization framework built on top. It is written for a
compiler engineer deciding which layout algebra fits their problem. The
primary source for data-structure conventions and pass design is
[`internal/subgroupchain/README.md`](../internal/subgroupchain/README.md);
this document links to it rather than duplicating it. For the other algebra
see [`doc/f2.md`](./f2.md) and the top-level [README](../README.md).

Prerequisites: familiarity with GPU threading (warps, lanes, memory
transactions) and mixed-radix (positional) number systems. Linear algebra
helps for reading the F₂ comparison but is not required here.

## 1. The math: Z/k sub-group chains

Take a finite cyclic group Z/M and a factorization M = m₀ · m₁ · … ·
mₖ₋₁ (the mᵢ need not be prime or powers of two). The chain of subgroups

```
Z/M ⊃ Z/(M/m₀) ⊃ Z/(M/(m₀m₁)) ⊃ … ⊃ {0}
```

gives every element of Z/M a **unique mixed-radix digit decomposition**:
the quotients hᵢ = x div (m₀…mᵢ₋₁) mod mᵢ. This is exactly how a
multi-dimensional array index decomposes into per-dimension coordinates,
generalized to arbitrary radices — the multi-dimensional version of the
same construction is a chain of sub-group chains, one per tile dimension.

The F₂ algebra is the special case where every radix is 2; there, digit
decompositions are bit decompositions and everything is linear algebra
over GF(2). The sub-group chain algebra keeps the digit machinery and
drops the linearity: it accepts **arbitrary integer factorizations**, which
is what makes 7×16×4 tiles and 96-element rows expressible at all.

## 2. The model

Three spaces, mirroring how a GPU kernel actually addresses memory (full
vocabulary in the package README):

- **Concurrency space — the bag of variables (BoV).** Instead of one
  linear induction variable, the concurrency space is a *set of named,
  bounded variables*: e.g. `warp` with bound 14, `lane` with bound 32.
  Order is **significant** (major → minor significance): within a
  dimension the first-consumed variable takes the most-significant digit,
  and across dimensions left is major. Put the lane variable **last** so
  its freedom trails into the low-weight tail — the position the GMEM/SMEM
  passes score against.
- **Coordinate space — hierarchical tuples (HTuple).** Tile coordinates
  are CuTE-style hierarchical tuples: a nested, denormalized tuple whose
  shape mirrors the tile's factorization, e.g. a 32×96 tile as
  `((4, 8), (16, 6))`. Nesting expresses sub-structure a flat vector
  cannot.
- **Offset space — one integer.** The stride map takes a coordinate HTuple
  to a byte offset by dot-product with the row-major stride vector
  (element offset × dtype size).

The two mappings:

**Access** (BoV → coordinate HTuple). Dimensions are processed left to
right; within a dimension, variables are scanned left to right and each
contributes `g = gcd(bound, remaining extent)` — the variable's low digit
(`v mod g`) joins the dimension's coordinate, the high part (`v / g`) is
carried to later dimensions. The mapping is accepted only if every
dimension is covered and every variable fully consumed — which is
guaranteed exactly when Π(bounds) == Π(extents). Trivial digits (bound 1)
are skipped. This is the sub-group chain digit decomposition run
backwards.

**StrideMap** (coordinate HTuple → byte offset). Recurses through nested
shapes mirroring the structure, validates dtype size (1/2/4) and
structural match, and applies row-major strides derived from the shape.

The expression IR underneath (see the package README for the exact node
set and invariants): Variable / Const / Symbol / Binary(Add/Sub/Mul/Div/Mod)
with truncating Go integer semantics, plus `Coordinate` — a **radix-based
extraction operator** `(source % radix) / stride` with bound `radix /
stride`, whose source may be a Variable (full or partial dedication),
another Coordinate (chained extraction), or a Stride (freedom pool) — and
`Stride`, an opaque dot-product node. Constructor canonicalization keeps
coord chains minimal, and a chain monotonicity invariant (UpperBound never
increases chasing sources) makes range analysis (`range.go`) sound.

## 3. Where conservatism enters

The F₂ algebra is exact: every answer is a provable rank computation. The
sub-group chain algebra trades exactness for expressiveness, under a hard
rule: **false negatives are acceptable; wrong math is not.**

- **Soundness is enforced loudly at construction**: BoV product must equal
  tile rank, every dimension fully covered, every variable fully consumed,
  coordinate/shape structural match. Violations are errors, never silent
  wrong mappings — the composed Access+StrideMap is a bijection onto
  [0, N) *by construction*, and property tests enumerate the full
  assignment space to prove it (see §5).
- **Optimization passes classify their failures**. The GMEM pass
  (`gmem_map.go`) bails with a *classified* verdict when no draw divisor
  fits the available freedom ("not representable" — mathematical), when
  the GCD chain stalls ("not extractable" — a two-stage draw would
  recover), or when a misaligned mixed digit blocks a nested last dim
  ("opaque mixed" — digit splitting would recover). In every classified
  case the layout is left untouched. The framework contract
  (`memory_optimization.go`): a pass that cannot optimize reports its
  verdict, never a silent no-op.
- **Analyses are interval-based and monotone** (`range.go`,
  `coord_analysis.go`): post-order data flow over the expr tree with a
  monotone interval lattice. Unknown = top; every computed bound is a
  conservative over-approximation used only to reject or guide, never to
  assert a false fact.

## 4. Worked example (verified program output)

Problem: tile `[2, 4]` of 4-byte elements; BoV `(warp w: bound 2, lane t:
bound 4)` — lane last, the order the passes like; Π(bounds) = 8 = Π(extents).

Access (low-digit peeling): dim 0 (extent 2): w contributes
g = gcd(2, 2) = 2, consuming w entirely. Dim 1 (extent 4): t contributes
g = gcd(4, 4) = 4, consuming t entirely. Coordinate = `(w, t)`; row-major
strides = (4, 1) elements. StrideMap emits the byte offset (× 4B):

```
(((((warp.w % 2) / 1) * 4) + (((lane.t % 4) / 1) * 1)) * 4)
```

The Coordinate operators read directly: `(warp.w % 2) / 1` is the low digit
of w in radix 2 (bound 2/1 = 2), `(lane.t % 4) / 1` is the low digit of t
in radix 4. Evaluating at every (w, t) — the full 8-point assignment space:

```
  (0,0) -> 0  (0,1) -> 4  (0,2) -> 8  (0,3) -> 12
  (1,0) -> 16  (1,1) -> 20  (1,2) -> 24  (1,3) -> 28
```

A bijection onto the byte multiples of 4 in a 32-byte tile, exactly as the
soundness invariant promises. The GMEM stage leaves this layout unchanged
(lane freedom already trails the last dimension): `gmem == base`.

For a case where the optimizer has real work, flip the BoV to lane-first on
a `[2, 96]` tile (`t` bound 32, `w` bound 6): the base mapping buries lane
freedom in the major dimension and each warp's 32 lanes scatter over 24
distinct 32B sectors; after `GmemOptimize` they share 4 sectors in one 128B
line. Reproducible via `go run ./examples/gmem-coalescing`.

## 5. Guarantees the code enforces

Every claim is backed by a test gate (paths relative to
`internal/subgroupchain` or `pkg/layout`); the suite is the oracle.

| Guarantee | Mechanism | Test gate |
|---|---|---|
| **Access+StrideMap is a bijection** — enforced construction invariants (Π(bounds) == Π(extents), full coverage, full consumption, structural match); violations are loud errors | `access_map.go`, `stride_map.go` | `soundness_test.go`; `map_test.go` exhaustively enumerates all 1400 (BoV, tile-shape) pairs with equal product N ≤ 60 over the full assignment space |
| **GMEM pass preserves bijectivity** and achieves the window objective on success; classified conservative errors otherwise | `gmem_map.go` framework instantiation | `gmem_test.go`: 1794 cases (flat + nested shapes, N ≤ 48) — bijection always, suffix ≥ W + exact sector counts on success |
| **SMEM pass preserves bijectivity**; every reported success is machine-checked conflict-free; multi-variable lane families rejected loudly | `smem_swizzle_map.go` framework instantiation | `smem_test.go`: 1021 single-lane cases |
| **Reference interpreter matches lowered ASTs** — `Eval` over the native IR equals `ast.Eval` over the lowered interchange AST on enumerated problems | `pkg/layout/lower.go` | `eval_test.go`, `pkg/layout` tests |
| **StrideMap validation** — dtype sizes 1/2/4, structural coordinate/shape match; mismatches are errors, not panics | `stride_map.go` | `tuple_test.go`, `htuple_test.go` |

## 6. Use cases, and the pending-work boundary

Use the sub-group chain algebra when:

- tiles or pitches are **not** powers of two (7×16×4, 96-wide rows,
  odd warp counts) — outside the F₂ model entirely;
- you want **CuTE-style hierarchical layout composition** (nested HTuples
  mirroring a recursive factorization);
- you need the **GMEM/SMEM optimization passes** that fix a mis-ordered BoV
  by drawing the lane family's freedom into the last dimension
  (`gmem_map.go`, `smem_swizzle_map.go`, built on the universal framework in
  `memory_optimization.go` — window → analysis → fast path → transform →
  coverage repair → post-check).

Pending work — do not assume these exist (see the package README's
"Not yet implemented" list):

- the **transvection swizzle** for SMEM: the `TrySwizzle` hook in the SMEM
  framework instantiation is the stage-4 slot and is **not implemented**;
  without a registered hook the pass falls back to reordering;
- a **folding / simplification engine** for the expression IR (unlike F₂'s
  canonical-form engine, sgc expressions are kept as-is);
- **reshape mapping** (merge/break) as explicit transformation passes;
- padding / truncation / injective-surjective support — by design;
  mismatched shapes are expected to be resolved upfront by the compiler.

## 7. Which algebra?

| Your problem | Choose |
|---|---|
| Arbitrary tile factorizations, nested/CuTE-style layouts, odd extents or warp counts | **sub-group chain** (`layout.SubgroupChain`) |
| Power-of-2 everything and you need *exact certified* rank/sector/bank answers | **F₂** (`layout.F2`) — see [`doc/f2.md`](./f2.md) |
| Padding, truncation, or mismatched shapes | neither — resolve alignment upfront and feed the aligned problem |

Both algebras share the same consumer facade (`pkg/layout`), the same
interchange AST (`pkg/ast`), and the same conservative contract: do what
you can, return a classified error when you cannot. The choice is a
one-token change (`layout.F2` ↔ `layout.SubgroupChain`) once the problem
satisfies the chosen algebra's preconditions (F₂: power-of-2 extents and
bounds with equal total bit rank; sub-group chain: Π(bounds) == Π(extents)
with the lane variable last for best optimization results).
