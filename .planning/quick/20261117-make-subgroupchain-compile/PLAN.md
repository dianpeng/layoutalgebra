---
type: quick
date: 2026-11-17
---

# Make internal/subgroupchain compile and work (stride + access maps only)

## Description

User added a new algebra package `internal/subgroupchain` (Z/k sub-group chain algebra,
CuTE-style HTuple layouts) with supporting additions in `internal/common`. The code is
WIP and does not compile. Scope: make existing functionality work — only the two
mappings (stride, access). Do NOT implement swizzle or a folding engine (user will add).

## Tasks

1. `internal/common`: fix `GCD` (drop x/exp dep, `bT` typo), fix `RemoveNil` return
   type, fix `Stride` (element type + reversed order), add `SliceEqual`.
2. `internal/subgroupchain/expr.go`: rename kind constants (`Const`→`KindConst`, ...)
   to avoid type-name collisions; fix `exprNode.Id/setId`; add missing `Kind()` methods;
   fix `Varaible` typo; fix `IsEqual` implementations (wrong var refs, missing length
   check); add `common` import.
3. `internal/subgroupchain/range.go`: fix `*Interval` → `*RangeInterval`; add
   `NewUnknown`/`NewInterval`; rename `AnalyseRange`→`AnalyzeRange` (match call site);
   fix Binary range transfer (Range has no `Kind()`; check `r.(*Const)` and
   `lR.(*RangeInterval)`).
4. `internal/subgroupchain/visit.go` (new): `VisitPost` walker (Binary l/r, Stride coords).
5. `internal/subgroupchain/factory.go`: add `Env` type; fix imports; fix `Variable`
   ctor (`newVar`→`Var{}`); fix `Factorize` (`<=1` bound check, pre-sized stride/out
   slices — both currently index into len-0 slices); fix `Product` return type.
6. `internal/subgroupchain/tuple.go`: fix `Rank` (NewBinary→Binary); implement
   `ToHTuple`/`ToConstTuple`; fix `ConstTuple.Gather` element type; add `Array()`;
   implement `Clone`; fix imports.
7. `internal/subgroupchain/htuple.go`: fix `sef`/`set.` typos, `valueA`, `self.len`,
   infinite-recursion `Height`, `Set*` arg mixups; implement `Clone` (deep); fix
   `Rank`/`Flatten`; add `NewHTuple`/`NewConstHTuple` constructors.
8. `internal/subgroupchain/stride_map.go`: fix recursion args (`.AsHTuple()`, env),
   `Stride(strideOffs)` call, `Value(dtypeSize)`, pass `dtypeSize` through `StrideMap`,
   fix return type `(Expr, error)`, leaf coord via `.Leaf()`.
9. `internal/subgroupchain/access_map.go`: fix imports, `AnalyzeRange` name,
   `rng.(*RangeInterval)`, error return tuples, nil-compaction (local, `RemoveNil`
   signature is []*T), error propagation from `doAccessMapOneDepth`, recursive
   `AsHTuple().Flatten()`.
10. Unit tests (stdlib only): common helpers; factory/kind/id; Factorize mixed-radix
    correctness; range analysis; access map end-to-end (README warp/lane → [7][16][4]
    example) verified by a small test-local evaluator; stride map end-to-end byte
    offsets; htuple flatten/clone/to-vector.

## Verification

- `go build ./...` and `go vet ./...` clean
- `go test ./...` passes (existing f2 tests must stay green)
