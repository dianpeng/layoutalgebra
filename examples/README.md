# Examples

Runnable consumer programs for the universal integration layer
(`pkg/layout` + `pkg/ast`). Each is stdlib-only and runs with `go run`:

- `compilerpass/` — the headline example: the consumer-side "pass" pattern.
  Wrap each stage's `ast.Expr` offset as a pass in your own pipeline,
  evaluate it with `ast.Eval` at concrete hardware coordinates, and feed
  the numbers to your cost model. Drives both algebras through the same
  facade (`go run ./examples/compilerpass`).
- `gmem-coalescing/` — focused GMEM coalescing on `layout.SubgroupChain`
  with a tuned `layout.WithLineBytes` window: per-warp 32B-sector and
  cache-line counts before/after `GmemOptimize`
  (`go run ./examples/gmem-coalescing`).
- `smem-swizzle/` — focused SMEM bank-conflict analysis on `layout.F2`
  with `layout.WithBankWindow`: distinct-banks-per-warp report via
  `ast.Eval`, plus both smem-stage outcomes (saturated success and the
  conservative-contract decline) (`go run ./examples/smem-swizzle`).

The singular `example/` is the end-to-end tour of both algebras (all
three stages, `ast.Dump`, conservative-error handling):
`go run ./example`.
