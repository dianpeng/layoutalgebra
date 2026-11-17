---
status: complete
date: 2026-11-17
---

# Summary: make internal/subgroupchain compile and work

## What was done

The user's new `internal/subgroupchain` package (Z/k sub-group chain algebra for CUDA
layout analysis) did not compile and had numerous latent bugs. Fixed everything needed
to make the existing functionality work — the `Access` and `StrideMap` mappings only.
No swizzle, no folding engine (user will add both).

## Fixes by file

### internal/common
- `gcd.go`: removed the `golang.org/x/exp/constraints` dependency (violates the
  near-zero third-party constraint), fixed the `bT` parameter typo, constrained on the
  repo's own `Integer`. Added `LCM`.
- `common.go`: `RemoveNil` was missing its return type; `Stride` mixed untyped `int`
  with `T` (did not compile) and produced the stride vector in reversed order — now
  returns row-major strides ([s1*s2*..., ..., 1]). Added `SliceEqual`.

### internal/subgroupchain
- `expr.go`: kind constants (`Const`, `Symbol`, `Variable`, `Binary`, `Stride`)
  collided with the struct type names — redeclared as `KindConst`/`KindSymbol`/
  `KindVariable`/`KindBinary`/`KindStride`. `exprNode.Id()` returned `marker`
  (aliasing the user marker with the node id) — now uses the `id` field. Added the
  missing `Kind()` methods for all five node types. Fixed `Varaible` typo. Fixed
  `IsEqual` implementations referencing the un-asserted `that` parameter instead of
  the typed local, and added the missing coordinate-length check in `Stride.IsEqual`.
  Clarified `Div` as truncating integer division (Factorize emits non-exact divs).
- `range.go`: `Mul`/`Div`/`Mod` referenced a nonexistent `Interval` type
  (`*RangeInterval`). Added `NewUnknown`/`NewInterval` constructors (referenced but
  never defined). Renamed `AnalyseRange` → `AnalyzeRange` (the access map called the
  latter). The `*Binary` transfer function called `rRng.Kind()` — `Range` has no
  `Kind()` — restructured to type-assert the operand expressions (`*Const` divider,
  `*RangeInterval` dividend) with unknown fallback.
- `visit.go` (new): `VisitPost`/`Visit` walker the range analyzer needs; descends
  `Binary` l/r and `Stride` coordinates.
- `factory.go`: added the `Env` type (referenced everywhere, defined nowhere). Fixed
  `Variable()` (`newVar` helper didn't exist). `Factorize` had three bugs: the
  "pointless factorization" check fired when the bound was *greater* than 1 (the
  normal case) instead of `<= 1`; `stride` and `out` were allocated with length 0 but
  indexed — panicked on first use; and for a trivial digit in a non-leading position
  the mod used a zero stride marker (division by zero) — now uses a separate radix
  array. `Factorize` now attaches ranges to generated digits (the access map queries
  the remainder's range on the next dimension — without this every multi-dimension
  access failed with "invalid range"). `Product` returned `*Binary` but built an
  `Expr` — return type is now `Expr`.
- `tuple.go`: `Rank` called a nonexistent `NewBinary` (`Binary`). Implemented the
  empty `ToHTuple`/`ToConstTuple`/`Clone`. `ConstTuple.Gather` used `[]Expr` for a
  `[]Value` slice. Added `Array()`. Removed unused imports.
- `htuple.go`: fixed six `sef` typos and three `set.` typos; `self.valueA` (leaf rank
  now type-asserts the stored `Value`); `self.len(...)` → `len(...)`; `Height()` on
  HTHTuple infinitely recursed (`self.Height()` → `1 + self.HTuple().Height()`);
  `Set*` methods passed the wrong variable. Implemented the empty `Clone()` deeply
  (value + htuple levels, tested for independence). Fixed `Rank`/`Flatten` type
  mismatches; added `NewHTuple`/`NewConstHTuple` and value constructors.
  `HTuple.Height()` semantics aligned with the README doc (top-level leaves = depth 0,
  +1 per htuple nesting).
- `stride_map.go`: recursion dropped `env` and passed a `*HTupleValue` where an
  `HTuple` was required (`.AsHTuple()`); leaf case passed the wrapper instead of
  `.Leaf()`; `Stride(strideOffs...)` treated a slice parameter as variadic;
  `value(dtypeSize)` used an undefined identifier; `StrideMap` dropped `dtypeSize` and
  declared the wrong return type (`HTuple` instead of `Expr`).
- `access_map.go`: fixed imports; `rng.(*Interval)` → `rng.(*RangeInterval)`;
  `return fmt.Errorf(...)` in a two-value function; `common.RemoveNil(bov)` has a
  `[]*T` signature that can't take `[]Expr` — compaction now done locally; error from
  `doAccessMapOneDepth` was dropped; the recursive path used a nonexistent
  `component.Flatten()` on the value — now `component.AsHTuple().Flatten()` with a
  `ToTuple()` conversion (added to `ConstHTuple`).

## Semantic note

The implementation peels the **low digit** of each variable per dimension
(`coord = variable % divider`, carried remainder = quotient). The README's worked
example peels warp's high digit first for dim 0. Both are valid bijections; the tests
assert the implementation's consistent low-digit semantics (bijection onto the full
tile verified by brute force).

## Review round (gsd-code-reviewer)

A review pass over the fixed code found and verified 4 further issues, all fixed:

- **C1 (critical)**: `Access` silently produced a non-injective mapping when the
  BoV product exceeded the tile rank — the leftover high digit was dropped and the
  `consumeFully` check was dead code (hard-coded `false`). Now: `Run` validates
  `Π bounds == tile rank` up front, `doAccessMapOneDepth` is wired with
  `consumeFully=true` and returns an error on leftover variables, and the
  per-dimension "not covered" invariant returns an error instead of panicking.
- **H1 (high)**: the recursive access-map path flattened the sub-tile, corrupting
  the coordinate structure for nesting depth ≥ 2 and panicking StrideMap. Now the
  sub-tile structure is passed through (`component.AsHTuple()`); covered by a
  depth-2 test `((2,(2,3)),4)` with brute-force bijection.
- **M1/M3 (medium)**: range transfer fixes — `Div` now computes `ceil(U/d)` (was
  floor, under-approximating), `Mod` computes `min(m, U)` (was `m`,
  over-approximating); non-positive dividers yield Unknown.
- **M2/L1/L2/L3/L4 (robustness)**: `Stride.Rank()`/tuple rank methods guard empty
  shapes (rank 1); `Factorize` validates non-empty bounds and entries ≥ 1;
  `Product()` on zero args returns Const(1); `Factory.Variable` requires
  `upperBound >= 1`; `GCD` takes operands by absolute value; `StrideMap` returns
  errors for structural mismatches and invalid dtype size instead of panicking
  (tests updated to expect errors).

## Verification

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./...` — all packages pass, including all pre-existing f2 tests

New tests (stdlib only): common helpers; factory nodes/ids/kinds/equality; Factorize
mixed-radix (basic, trivial middle/leading digits, all error paths); range analysis
(incl. ceil-div and over-large-modulus transfers); HTuple/Tuple flatten/clone/height/
conversions with deep-clone independence checks; Access end-to-end on the README
example (448-element bijection brute force); Access on nested htuple tiles of depth 1
and 2 (recursion paths); Access error paths (BoV/tile rank mismatch, incompatible
tile, bound-1 variable); StrideMap dtype scaling and invalid-dtype rejection.
