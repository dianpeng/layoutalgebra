// Package layoutalgebra is the umbrella for multiple symbolic layout-algebra
// implementations used to analyze GPU memory layouts, and the universal
// integration interface for upper-layer consumers.
//
// The module is organized in three layers:
//
//  1. This package — the universal interface. Upper layers (a compiler, a
//     layout planner, an interactive user) integrate against [Algebra] only;
//     they pick an implementation by name or capability and drive the pipeline
//     stages without knowing the algebra underneath.
//
//  2. The algebra implementations (today: the bit-level F₂ algebra in
//     internal/f2 for power-of-2 layouts; the Z/k sub-group chain algebra in
//     internal/subgroupchain for arbitrary tile factorizations with CuTE-style
//     hierarchical tuples). Each is self-contained and exposes its own native
//     API beside the universal one.
//
//  3. Per-algebra AST/code generation (not implemented yet). Once all algebras
//     are complete, each grows a code-generation backend that interprets its
//     native mapping artifacts into emitted index arithmetic. That layer plugs
//     in behind the opaque [Mapping] / [Offset] handles returned here, making
//     this package the single integration point: one consumer, many algebras,
//     many backends.
//
// Contract rules for implementations
//
//   - Conservative, per the project invariant: do what you can, return an
//     error when you cannot. False negatives are acceptable; wrong math and
//     silent failures are not.
//   - Implementations must not mutate the *Problem they are handed.
//   - The results (Mapping / Offset) are opaque outside the producing
//     algebra; only its own pipeline stages and its future code-generation
//     backend may interpret Value.
package layoutalgebra
