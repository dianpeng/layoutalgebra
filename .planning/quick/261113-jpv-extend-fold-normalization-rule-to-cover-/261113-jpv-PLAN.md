---
phase: quick-261113-jpv
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/f2/fold.go
  - internal/f2/fold_test.go
autonomous: true
requirements: [ALG-03]
must_haves:
  truths:
    - "Fold(x + S*x) and Fold(S*x + x) both produce the same pinned canonical tree Mul(Add(One, S), x)"
    - "Fold does NOT merge Add(v, Mul(c, w)) when v and w are different variables, and leaves Add(v, w) untouched"
    - "CoeffMatAll on a vector containing x + M*x yields a single coefficient equal to the folded (1+M) expression, not silently M"
    - "Constant-coefficient merges fold fully: x + ConstOne*x collapses to 0; a const c merges to a correct constant coefficient"
    - "Regression locks hold: M*x + N*x -> Mul(Add(M,N), x) and x + x -> 0 are unchanged"
    - "The fold engine reaches a fixpoint deterministically (no rule oscillation) under a -count=20 run"
  artifacts:
    - path: "internal/f2/fold.go"
      provides: "rewriteNormalization handling both bare-variable operand orders with static-coefficient guard"
      contains: "IsStaticExprRecursive"
    - path: "internal/f2/fold_test.go"
      provides: "RED-first regression tests pinning the new canonical trees and the certificate-soundness lock"
      contains: "TestFoldNormalizationBareVariable"
  key_links:
    - from: "internal/f2/fold.go rewriteNormalization"
      to: "internal/f2/fold.go rewriteRuleOrder"
      via: "new cases inserted into the existing normalization rule; driver order unchanged"
      pattern: "rewriteRuleOrder"
    - from: "extractLinearCoeffVector (vector.go)"
      to: "Fold"
      via: "doCoeffMat folds before extraction since WR-01; the new normalization case merges x + M*x so extraction no longer silently drops the bare x"
      pattern: "Fold\\("
---

<objective>
Extend the fold engine's rewriteNormalization rule so the bare-variable coefficient-1 special case merges: `C1*v + C2*v => (C1+C2)*v` must also fire when one side is a bare variable (C=1). This fixes the `x + S*x` silent-drop in coefficient extraction at the fold layer, per the known limitation documented in the 261113-gll SUMMARY.

Purpose: `extractLinearCoeffVector` relies on Fold to canonicalize per-variable closures. Today Fold leaves `x + S*x` unmerged, and the extractor's `exprMap[name] = e` overwrite silently keeps only the `S*x` closure, returning coefficient M instead of M+1 — a silent wrong answer in the library's core value path. Fixing it in the fold engine (user's explicit direction) keeps vector.go untouched.
Output: Two new normalization cases in rewriteNormalization + RED-first tests pinning the canonical trees, regression locks, and the CoeffMatAll certificate-soundness test.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@./CLAUDE.md
@internal/f2/fold.go
@internal/f2/fold_test.go
@internal/f2/pipeline_test.go

<interfaces>
Key constructors and helpers (from internal/f2/expr.go):
- NewBinary(op int, l, r Expr) *NodeBinary — also NewAdd(l,r), NewMul(l,r)
- NewConstZero(), NewConstOne() *NodeConst; NewSymbol(s string) *NodeSymbol; NewVariable(s string) *NodeVariable
- IsStaticExpr(e) bool — Kind()==Const || Kind()==Symbol (non-recursive)
- IsStaticExprRecursive(e) bool — recursive static check (expr.go line 230)
- Expr.IsEqual(other Expr) bool — structural/representational equality
- Kind() values: Const, Symbol, Variable, Binary; Op values: Add, Mul

Existing rewriteNormalization (fold.go ~line 268): fires only when bin.L() and bin.R() are BOTH *NodeBinary with Op()==Mul, both coefficients IsStaticExprRecursive, and the variable operands IsEqual — merging to NewBinary(Mul, NewBinary(Add, a, c), b). Your new cases slot into the SAME Rewrite method, after the existing both-Mul case.

Fixed rule order (rewriteRuleOrder, fold.go line 67): rewriteFold -> rewriteCommute -> rewriteIdempotent -> rewriteReassociation -> rewriteDistribution -> rewriteNormalization -> rewriteSymbol. The driver iterates this slice and applies the FIRST matching rule, then restarts from the top on the replaced tree.
</interfaces>

<design_pins>
1. New case A — `Add(v, Mul(c, w))` with bin.L().Kind()==Variable, bin.R() a Mul whose L is IsStaticExprRecursive and whose R satisfies w.IsEqual(v): rewrite to NewBinary(Mul, NewBinary(Add, NewConstOne(), c), v).
2. New case B — `Add(Mul(c, w), v)` symmetric: bin.R().Kind()==Variable, bin.L() a Mul with static coefficient c and Mul.R().IsEqual(v): rewrite to NewBinary(Mul, NewBinary(Add, c, NewConstOne()), v).
   Handle BOTH orders explicitly. Do NOT rely on rewriteCommute having normalized the order — an Add with a bare Variable on one side and a Mul on the other may not be commuted by rewriteCommute (its case #1 only commutes when R is fully static; a Mul is not), so each order needs its own branch.
3. Guard conditions (must all hold to fire): outer op is Add; the bare side Kind()==Variable (NOT Symbol, NOT Binary); the Mul side Op()==Mul; Mul's coefficient (the non-variable side — check both Mul operand positions and require exactly the coefficient side IsStaticExprRecursive while the other side IsEqual the bare variable; simplest correct form: Mul.L() IsStaticExprRecursive AND Mul.R().IsEqual(bareVar)); the Mul's variable operand IsEqual the bare variable. If the Mul's variable differs from the bare variable, leave untouched. `Add(v, w)` (two bare variables) is untouched. Do NOT add a case where the Mul is on a matching variable but the coefficient is non-static — that shape stays as-is (matches the existing both-Mul rule's static requirement).
4. Canonical tree expectation (executor MUST probe empirically with a scratch program and pin exactly what the fixed-order driver produces; both operand orders must converge to the SAME tree):
   - Symbolic c=S: coefficient Add(One, S). rewriteFold cannot reduce it (1 xor S unknown); rewriteSymbol keeps const-left/symbol-right, so it stays Add(One, S). Expected: Mul(Add(One, S), x) — i.e. NewMul(NewAdd(NewConstOne(), S), x) — for BOTH Fold(x + S*x) and Fold(S*x + x).
   - Constant c=ConstOne: the merged coefficient Add(One, One) folds via rewriteFold to NewConstZero(); then Mul(0, v) folds to 0 via rewriteFold case #3. Expected final: NewConstZero().
   - Constant c=ConstZero cannot survive to normalization as a Mul coefficient — rewriteFold identity already collapsed Mul(0,v) to 0 before normalization runs; if your probe shows otherwise, assert what the driver actually does.
   - A distinct constant c (e.g. ConstOne is the only non-zero F2 constant — F2 has only 0 and 1, so the only constant merge is the c=1 case above; do not invent other constants).
5. Rule-interaction safety (no oscillation):
   - rewriteDistribution fires BEFORE normalization on Mul(Add(a,b), c) but only when BOTH a and b are non-static (!IsStaticExprRecursive). The merged coefficient Add(One, S) has a static operand, so distribution never re-expands the merge — no Mul/Add cycle.
   - rewriteIdempotent runs before normalization: Add(x, Mul(c,x)) is never l.IsEqual(r) (different kinds), and after the c=1 identity collapse Add(x,x) hits idempotent first and returns 0 before normalization sees it — so the x+x→0 lock is preserved.
   - All merged nodes are freshly constructed via NewBinary/NewConstOne — the tree-never-DAG invariant holds; never mutate the input node in place for the new cases.
   - If any oscillation exists, the driver hard-aborts at the iteration cap via check.Bug (WR-10) — the -count=20 determinism run in verify catches this loudly.
</design_pins>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Extend rewriteNormalization with bare-variable merge (RED then GREEN)</name>
  <files>internal/f2/fold_test.go, internal/f2/fold.go</files>
  <behavior>
    - TestFoldNormalizationBareVariable: Fold(NewAdd(x, NewMul(S, x))) and Fold(NewAdd(NewMul(S, x), x)) BOTH IsEqual NewMul(NewAdd(NewConstOne(), S), x) (or the exact tree your probe of the fixed-order driver produces — pin it verbatim). x=NewVariable("x"), S=NewSymbol("S").
    - Negative guards: Fold(NewAdd(x, NewMul(S, y))) where y=NewVariable("y") is structurally unchanged (still an Add with a Variable L and Mul R); Fold(NewAdd(x, y)) unchanged (still Add of two Variables); Fold(NewAdd(x, NewMul(S, NewAdd(x, y)))) (non-static-coefficient shape) unchanged.
    - Constant merge: Fold(NewAdd(x, NewMul(NewConstOne(), x))) == NewConstZero(); Fold(NewAdd(NewMul(NewConstOne(), x), x)) == NewConstZero().
    - Regression (pre-existing locks must stay green): Fold(NewAdd(NewMul(M,x), NewMul(N,x))) IsEqual NewMul(NewAdd(M,N), x); Fold(NewAdd(x, x)) == NewConstZero().
    - Certificate-soundness integration lock: v := NewVector([]Expr{NewBinary(Add, x, NewBinary(Mul, M, x))}); mat, err := v.CoeffMatAll(); err==nil; mat is 1x1 (one row, one basis variable x); mat.Index(0,0) IsEqual NewAdd(NewConstOne(), M) (probe-confirmed: the extractor returns coeffExpr = node.L(), the coefficient WITH the variable stripped — the merged form Mul(Add(One,M), x) yields the entry Add(One, M), NOT the whole Mul) — NOT bare M. (If the folded shape differs after your probe, compare against Fold of the same sub-expression so the lock tracks the engine's canonical form — the essential assertion is the coefficient contains the merged (1+M), not silently M.)
  </behavior>
  <action>
    RED: Write the tests above into internal/f2/fold_test.go (append after TestFoldNormalization; name the new tests TestFoldNormalizationBareVariable, TestFoldNormalizationBareVariableNegative, TestFoldNormalizationBareVariableConst, TestFoldNormalizationCertificateSoundness). Run `go test ./internal/f2/ -run 'TestFoldNormalization' -count=1` and confirm the new tests FAIL — the bare-variable cases currently return the unfolded Add; for the certificate test, PROBE the actual pre-fix failure mode yourself (plan-checker probe: a CHECK panic at the variable-kind check in extractLinearCoeffVector; an earlier probe observed a silent wrong coefficient — either way the test must fail on pre-fix code; capture whichever truly happens and record it in SUMMARY.md). Do NOT weaken the test to accommodate the pre-fix failure. Commit RED: `test(261113-jpv): add failing tests for bare-variable normalization`.

    GREEN: Extend rewriteNormalization.Rewrite in internal/f2/fold.go ONLY (vector.go stays untouched per user direction). After the existing both-Mul block, add the two bare-variable branches per design_pins 1-3: (A) bin.L().Kind()==Variable && bin.R() is *NodeBinary with Op()==Mul && IsStaticExprRecursive(mul.L()) && mul.R().IsEqual(bin.L()) -> return NewBinary(Mul, NewBinary(Add, NewConstOne(), mul.L()), bin.L()); (B) bin.R().Kind()==Variable && bin.L() is *NodeBinary with Op()==Mul && IsStaticExprRecursive(mul.L()) && mul.R().IsEqual(bin.R()) -> return NewBinary(Mul, NewBinary(Add, mul.L(), NewConstOne()), bin.R()). Construct all nodes fresh (no in-place Commute on the input). Keep the existing both-Mul case unchanged. If a probe shows your expected canonical tree in the tests is wrong, correct the TEST to the actual driver output and record the pinned tree in the test comment — never weaken the guards.

    Do not touch rewriteRuleOrder, the driver, or any other rule. Commit GREEN: `feat(261113-jpv): extend fold normalization to bare-variable coefficient-1 case`.
  </action>
  <verify>
    <automated>go build ./... && go vet ./... && go test ./... -count=1 && go test ./internal/f2/ -count=20 -run 'TestFold|TestPipeline'</automated>
  </verify>
  <done>
    Both operand orders of x + S*x and S*x + x fold to the same pinned canonical tree; different-variable and non-static-coefficient shapes are untouched; x + 1*x folds to 0; M*x + N*x and x + x regressions stay green; CoeffMatAll on [x + M*x] yields a single coefficient equal to the folded (1+M) expression; full build/vet/test suite green plus a -count=20 determinism run with no check.Bug aborts.
  </done>
</task>

</tasks>

<verification>
- `go build ./... && go vet ./... && go test ./... -count=1` all green.
- `go test ./internal/f2/ -count=20 -run 'TestFold|TestPipeline'` green — fixed-order driver determinism, no iteration-cap abort (WR-10 loud-failure check).
- Grep gate: `grep -v '^[[:space:]]*//' internal/f2/fold.go | grep -c "NewConstOne"` returns >= 3 (identity rewrite already had none; the two new branches add uses — adjust only if the actual count differs but must be > 0) AND `grep -c "exprMap\[name\] = e" internal/f2/vector.go` unchanged (vector.go untouched).
</verification>

<success_criteria>
- rewriteNormalization merges C1*v + C2*v when C1 or C2 is the bare variable 1, in both operand orders, converging to one canonical tree.
- No silent drops: the CoeffMatAll certificate test proves x + M*x yields coefficient (1+M).
- All pre-existing fold/pipeline locks (TestFoldNormalization, idempotent, pipeline tests) remain green; -count=20 run confirms fixpoint termination.
- vector.go is not modified.
</success_criteria>

<output>
Create `.planning/quick/261113-jpv-extend-fold-normalization-rule-to-cover-/261113-jpv-SUMMARY.md` when done
</output>
