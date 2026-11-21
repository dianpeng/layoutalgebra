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
//  3. The universal integration layer (pkg/ast + pkg/layout). pkg/layout is
//     the abstract factory: the consumer picks an algebra by kind, drives the
//     pipeline stages, and receives algebra-agnostic ast.Expr artifacts —
//     importing it also registers every shipped algebra in this package's
//     registry. pkg/ast is the sealed interchange tree a compiler backend
//     translates into its own IR (never interpreting algebra-native types).
//     The per-algebra backends that emit index arithmetic from that AST are
//     the remaining piece of the "one consumer, many algebras, many
//     backends" endgame.
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
