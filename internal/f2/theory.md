# Fold Engine: Theory Model

This document is the formal model for the fold engine in `fold.go` — the
rewrite system, its lattice interpretation, the termination and confluence
arguments, and the executable evidence that backs them (the test gates in
`canon_test.go` and `convergence_test.go`).

## 1. Syntax

Expressions `E` over:

- variables `V` (basis bits, e.g. lane bits),
- constants `{0, 1}`, symbols `Σ` (static parameters, e.g. `M`, `N`),
- binary operators `⊕` (Xor/Add) and `⊗` (Mul/And), both commutative and
  associative over F₂.

Expressions are trees, never DAGs (project invariant). The static fragment
(constants + symbols) is closed under both operators.

## 2. The rewrite system

Six left-linear rules over binary nodes, iterated by a run-to-fixpoint driver
(`folder.run`), which is **chaotic iteration** over the tree: one replacement
per pass until a full pass fires nothing.

| rule | schema (local) |
|---|---|
| `rewriteFold` | constant identities: `0⊕x→x`, `1⊗x→x`, `0⊗x→0`, `c⊕c'→…`, `c⊗c'→…` |
| `rewriteCommute` | (1) pair order: `(A op B) → (B op A)` when `B < A` in the global order, except when the flatten pass owns the node; (2) same-op component: flatten → sort by the global order → rebuild left-nested |
| `rewriteIdempotent` | `x⊕x→0`, `x⊗x→x` |
| `rewriteReassociation` | rotation `A op (B op C) → (A op B) op C`; flatten `(A op B) op (C op D) → ((A op B) op C) op D` |
| `rewriteDistribution` | `(a⊕b)⊗c → (a⊗c)⊕(b⊗c)` **and** `c⊗(a⊕b) → (c⊗a)⊕(c⊗b)` for non-static `a, b` |
| `rewriteNormalization` | `c₁⊗v ⊕ c₂⊗v → (c₁⊕c₂)⊗v` and the bare-variable case `v ⊕ c⊗v → (1⊕c)⊗v` (both operand orders); also fires on common non-variable factors (factoring) |

### Global order

`OTuple` (see `order.go`) is a structural, total, lexicographic order:
kind rank `Const < Symbol < Variable < Binary`, then const value, name, or
`(op, lhs, rhs)` recursively. No hashing anywhere — structurally distinct
expressions always compare strictly, so sorting is deterministic.

**Canonical form** = no same-op right-nesting; every same-op component
flattened and order-sorted; no rule redex remaining. The canonical-form
predicate is checked by the enumeration gate (`TestFoldCanonicalEnumeration`).

## 3. Lattice interpretation

The user's framework, made precise: folding is a **dataflow analysis** over
the expression tree (Cousot–Cousot style; the driver is chaotic iteration).

- **Lattice elements**: per-node abstract states — order tuple, leaf-variable
  set, component membership, sortedness. States form a product lattice,
  ordered pointwise; height is finite per tree.
- **Within `()` (basic block)**: the transfer function — each rule is a local
  refinement `f(parent, children states) → parent state`. Every rule must
  declare PRE (local shape), POST, and which measure coordinate it decreases.
- **Cross `()` (propagation)**: a child rewrite changes the parent's input
  state, so the parent is re-evaluated — the dependency edge. The fixpoint
  driver is the worklist. The 3-window sort livelock (fixed in
  `f38cade`) was exactly a convergence failure of this layer: a local
  transfer (sort 3) that violated an ancestor invariant (left-nesting) with
  no shared lower bound — repaired by making the transfer own the whole
  component.

The extraction layer's markers are the same idea in ad-hoc form: NOV = an
abstract value, the marking pass = running the analysis, the linearity check
= querying it.

## 4. Termination

Each rule strictly decreases **its own witness** on every firing — checked
per rule application over all 10,788 enumerated expressions
(`TestFoldMeasureDecreases`):

| rule | witness | why it strictly decreases |
|---|---|---|
| `rewriteFold`, `rewriteIdempotent` | node count `N` | every firing removes nodes |
| `rewriteNormalization` | lexicographic `(P, N)`: `P` = per Add-component, per-variable C(k,2) over elements whose **leaf set** contains the variable; `N` | variable merges drop `P`; factoring (common non-variable factor) drops `N` instead. `P` is defined on leaf sets, which commute and reassociation provably preserve |
| `rewriteReassociation` | right-edge potential `R`: per same-op right edge at component depth `dp` in a component of height `H`, contributes `H − dp` | rotation moves an edge strictly deeper or eliminates it; **no rule ever creates a same-op right edge** (the flatten rebuild is left-nested; swaps only remove). Plain edge *count* is insufficient — rotation relocates an edge without changing the count |
| `rewriteCommute` case #1 | depth-weighted inversions `Iw`: an inversion in a component at depth `dm` contributes `w^dm`, `w = 2·n₀+1` fixed per fold | the swap leaves every depth unchanged; it removes its own component's inversion and can flip at most `n₀` inversions per ancestor level — `w`'s geometric growth makes the removal dominate all additions. (Unweighted global inversions are **not** monotone: swapping children changes the node's tuple, and siblings keyed between old and new tuple turn sorted pairs into inversions) |
| `rewriteCommute` case #2 | the fired component's own inversion count → 0 | firing requires unsortedness; the rebuild touches only the component's nesting, never element subtrees, so ancestor keys and sibling tuples are unchanged |
| `rewriteDistribution` | arithmetic interpretation: leaves 1, `⊕(a,b) → a+b+1`, `⊗(a,b) → 2ab` | each step `((x⊕y)⊗z) → (x⊗z)⊕(y⊗z)` decreases it by `2|z| − 1 ≥ 1` |

**Honest caveat**: a *single* well-founded measure decreased by the union of
all rules is the classical AC-normalization termination problem and is not
attempted here; the per-rule witnesses plus the WR-10 iteration-cap abort
(loud failure, never a silent non-canonical result) are the practical
guarantee, and the enumeration gate verifies convergence empirically at
depth ≤ 4 for every input shape simultaneously.

## 5. Confluence

Newman's lemma: a terminating rewrite system is confluent iff it is locally
confluent. Local confluence is checked empirically by **rule-order
permutation** (`TestFoldConfluence`): folding each enumerated expression
under randomly permuted rule orders must yield structurally identical
canonical forms. Two genuine violations were found and fixed:

1. **`rewriteSymbol` deleted** — it commuted Symbol/Const pairs *in place* and
   returned `nil`, so the driver's change detection saw "no replacement" and
   stopped one rewrite short of the fixpoint (`(S ^ (0*x))` stalled at
   `(0 ^ S)` under permuted orders). Fully subsumed by the global order.
2. **Distribution matched only `Add ⊗ c`** — `rewriteCommute` could hide a
   left-side redex by swapping before distribution saw it, making the
   canonical form order-dependent (`(x * (x^y))` vs `(x ^ (x*y))`). Matching
   non-static Adds on **either side** makes the redex shape-driven; the
   commute-vs-distribution conflict disappears because both orientations
   distribute.

## 6. Completeness

- **Enumeration gate**: all expressions up to 4 leaves over `{x, y, S, 0}` ×
  both operators (10,788) fold to the canonical form, idempotently.
- **Lifting argument**: every rule inspects at most a node and its children
  (context depth ≤ 2), so rule interactions arise only within bounded
  neighborhoods. Bounded-context rules + joinable critical pairs + decreasing
  witnesses imply completeness at *every* depth from finite checking — the
  enumeration gate is the empirical half of that certificate; critical-pair
  enumeration is future work.
- **Consequence already visible**: the former issue-B false negatives
  dissolve at the fold layer — `x + S·(x^y)` now folds to the canonical
  linear form `(S·y) ⊕ (x·(1⊕S))` and extracts to coefficients `[(1+S), S]`.

## 7. Contract

`Fold(e)` terminates (or aborts loudly at the cap), and for every expression
in the verified fragment returns the unique canonical form: fully sorted,
single-closure-per-variable linear normal form, independent of rule
application order. Downstream consumers (coefficient extraction, codegen)
may rely on this as the engine's contract.
