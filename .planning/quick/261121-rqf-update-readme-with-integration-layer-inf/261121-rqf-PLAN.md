---
phase: quick-261121-rqf
plan: 01
type: execute
wave: 1
depends_on: []
files_modified: [example/main.go, README.md]
autonomous: true
requirements: [quick-261121-rqf] # quick task — no ROADMAP requirement IDs; the quick-task id is the requirement

must_haves:
  truths:
    - "`go run ./example` exits 0 and prints an F2 section then a sub-group-chain section, each stage artifact as a deterministic one-line AST (String), one multi-line ast.Dump block, and two ast.Eval byte-offset values"
    - "The F2 [4,8] SmemOptimize conservative error is printed informationally and execution continues (the [4,32] block follows) — error handling per the conservative contract is demonstrated, not fatal"
    - "go build ./... && go vet ./... && go test ./... -count=1 pass with example/ included in the module build"
    - "README.md documents the runnable example (run command + project-layout tree entry), the integration-layer import paths, and the per-algebra variable-naming convention for ast.Eval — every claim verifiable against code, no aspirational language"
  artifacts:
    - path: "example/main.go"
      provides: "Runnable end-to-end example of the pkg/layout facade + pkg/ast artifacts for both algebras"
      min_lines: 80
    - path: "README.md"
      provides: "Extended universal-integration docs: example pointer, import paths, Eval naming convention"
      contains: "go run ./example"
  key_links:
    - from: "example/main.go"
      to: "pkg/layout"
      via: "layout.New + Engine.NewSession + Session stage methods"
      pattern: "layout\\.New\\(layout\\.(F2|SubgroupChain)\\)"
    - from: "example/main.go"
      to: "pkg/ast"
      via: "String()/ast.Dump/ast.Eval on Mapping.Offset"
      pattern: "ast\\.(Dump|Eval)\\("
    - from: "README.md"
      to: "example/"
      via: "run instruction and project-layout tree entry"
      pattern: "go run \\./example"
---

<objective>
Update README.md with the new universal-integration-layer information and add a runnable
example under example/ that illustrates using the library end to end: pick an algebra
(`layout.F2` vs `layout.SubgroupChain`), build a `layoutalgebra.Problem`, drive
Access / GmemOptimize / SmemOptimize on a Session, and print/dump the resulting
algebra-agnostic ASTs — including one stage whose conservative error must be handled
gracefully.

Purpose: Quick task 261121-om7 landed `pkg/ast` + `pkg/layout` (commits 55c0b44..c23a066)
but there is no runnable consumer example and no usage documentation for it beyond the
inline README snippet. This task closes that gap so a new user can `go run ./example`
and see the whole facade work.

Output: `example/main.go` (new, package main, stdlib only) and an extended README.md.
No changes to `algebra.go`, `pkg/`, or `internal/` source.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@README.md
@pkg/layout/layout.go
@.planning/quick/261121-om7-create-universal-integration-interfaces-/261121-om7-SUMMARY.md

<interfaces>
<!-- Verified API surface and known-good fixtures. Use these directly — no codebase
     exploration needed. All signatures checked against source on 2026-11-21. -->

From pkg/layout (facade):
```go
func New(kind Kind, opts ...Option) (*Engine, error)   // layout.F2 | layout.SubgroupChain
func (e *Engine) NewSession(p layoutalgebra.Problem) (*Session, error) // validates + Supported gate
func (s *Session) Access() (*Mapping, error)
func (s *Session) GmemOptimize() (*Mapping, error)
func (s *Session) SmemOptimize() (*Mapping, error)     // conservative: may return an error verdict
type Mapping struct { Algebra Kind; Access ast.Expr; Offset ast.Expr }
```
Options (`WithLineBytes`, `WithBankBytes`, `WithBankWindow`) — the example uses DEFAULTS
only; fixture behavior below is verified under defaults. Mention options in a comment.

From the root package (algebra.go):
```go
type Problem struct { Tile Tile; Concurrency []Variable; Lane string }
type Tile struct { Shape []int; DTypeSize int }   // DTypeSize must be 1, 2, or 4
type Variable struct { Family string; Name string; Bound int } // values in [0, Bound)
```

From pkg/ast:
```go
func Dump(e Expr) string                                   // multi-line prefix form, 2-space indent
func Eval(e Expr, assignment map[string]int) (int, error)  // reference integer evaluator
e.String()                                                 // infix, fully parenthesized, deterministic
```
`Eval` errors on: Var/Symbol with no assignment entry, div/mod-by-zero, Tuple (structural).

**Variable naming in lowered ASTs (critical, differs per algebra):**
- F2: per-BIT variables — bit j of variable v is named `v.Name + j` (e.g. w0, w1, t0..t2),
  Family empty. A variable-level assignment {w:2, t:5} must be expanded to bit level
  before `ast.Eval` (mirror `expandBits` in pkg/layout/layout_test.go:67).
- Sub-group chain: variable-LEVEL names (w, t) directly — `ast.Eval(offset,
  map[string]int{"w": 3, "t": 17})` works as-is (README snippet shows exactly this).

**Known-good fixtures — lift EXACTLY, do not invent problems (all verified in the
pkg/layout test suite):**

| id | Tile | Concurrency (order matters) | Lane | Verified stage outcomes |
|----|------|------------------------------|------|-------------------------|
| f2A | [4,8] d4 | {warp,w,4},{lane,t,8} | t | Access OK, Gmem OK, Smem ERRORS (no-free-degree) — pkg/layout/layout_test.go:15, f2_test.go:235 |
| f2B | [4,32] d4 | {warp,w,2},{lane,t,64} | t | Smem OK (clean saturation) — layout_test.go:29, f2_test.go:202 |
| sgcA | [7,16,4] d4 | {warp,w,14},{lane,t,32} — lane LAST (sgc BoV order) | t | Access OK, Gmem OK, Smem OK — subgroupchain_test.go:19, layout_test.go:170 (smoke test) |
| sgcB | [2,96] d4 | {lane,t,32} FIRST, {warp,w,6} — lane-first BoV | t | Gmem actually REWRITES the mapping (draw path) — subgroupchain_test.go:46 |

Repo facts: module `github.com/dianpeng/layoutalgebra` (go 1.26.2); no example/ dir yet;
`scripts/format.sh` only gofmt-checks `internal/` — it does NOT cover example/, so the
example task must gofmt-check itself. Go stdlib only (project constraint).
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create example/main.go — runnable tour of both algebras</name>
  <files>example/main.go</files>
  <action>
Create the directory `example/` with a single file `example/main.go`, `package main`,
Go stdlib only (`fmt`, `os`), plus the three project imports `github.com/dianpeng/layoutalgebra`,
`github.com/dianpeng/layoutalgebra/pkg/ast`, `github.com/dianpeng/layoutalgebra/pkg/layout`.
Target ~120-160 lines, small functions (repo style: functions under 50 lines).

Structure: `main` calls `f2Demo()` then `sgcDemo()`; one helper `expandBits(p
layoutalgebra.Problem, assign map[string]int) map[string]int` that extends a
variable-level assignment with per-bit entries (bit j of variable v named `v.Name+j`,
value `(assign[v.Name]>>j)&1`) — same convention as pkg/layout/layout_test.go:67; and a
`fatalf` helper (`fmt.Fprintf(os.Stderr, ...); os.Exit(1)`) for UNexpected failures.
Expected stage errors are printed informationally and execution continues — that is the
conservative contract being demonstrated.

f2Demo — engine via `layout.New(layout.F2)` with default options only (comment noting
`WithLineBytes`/`WithBankBytes`/`WithBankWindow` exist):
1. Problem f2A (exact fixture above). Print the section header exactly
   `=== F2 algebra (layout.F2) ===` then a problem line
   `problem: tile [4,8] dtype 4B, warp w bound 4, lane t bound 8`.
   NewSession; `Access()` and `GmemOptimize()` each print
   `Access offset: <Offset.String()>` / `GmemOptimize offset: <Offset.String()>`
   (one line each). Then an Eval demo on the gmem Mapping: expand {w:2, t:5} via
   expandBits, `ast.Eval(gmem.Offset, bits)`, print `byte offset at w=2 t=5: <n>`.
2. Same session: `SmemOptimize()` — this RETURNS an error on f2A. Print
   `SmemOptimize: not applicable to this tiling (conservative contract): <err>` and
   CONTINUE (no exit). This is the documented error-handling demonstration.
3. Problem f2B on the same engine, new session, problem line
   `problem: tile [4,32] dtype 4B, warp w bound 2, lane t bound 64`;
   `SmemOptimize()` succeeds — print `SmemOptimize offset: <Offset.String()>`.

sgcDemo — engine via `layout.New(layout.SubgroupChain)`:
1. Problem sgcA (lane variable LAST in the Concurrency slice). Print header exactly
   `=== sub-group chain algebra (layout.SubgroupChain) ===` and problem line
   `problem: tile [7,16,4] dtype 4B, warp w bound 14, lane t bound 32`.
   All three stages print their Offset String lines (`Access offset:` etc.). Eval demo
   on the gmem Mapping with the variable-level map {"w": 3, "t": 17} — NO bit expansion
   for sgc — print `byte offset at w=3 t=17: <n>`. One Dump illustration: print
   `GmemOptimize offset tree (ast.Dump):` followed by `ast.Dump(gmem.Offset)`
   (multi-line, indented).
2. Problem sgcB (lane variable FIRST — this ordering is the point of the fixture).
   Problem line `problem: tile [2,96] dtype 4B, lane t bound 32, warp w bound 6`.
   One session, `GmemOptimize()` prints `GmemOptimize offset: <Offset.String()>`
   followed by a short plain-language note line that the optimizer rewrote the mapping
   (drew lane freedom into the last dimension) because the lane-first ordering lost
   coalescing.

Determinism rules: print only via String()/Dump()/Eval on fixed assignments — never
addresses, timings, or map iteration order. Output must be byte-identical across runs.

Guardrail: if any stage this plan marks as success unexpectedly errors (or vice versa),
STOP and verify against the cited test before proceeding — do not silently swap fixtures.
  </action>
  <verify>
    <automated>cd /home/ps/Workspace/cuda/project/f2algebra && go build ./... && go vet ./... && go test ./... -count=1 && [ -z "$(gofmt -l example/)" ] && go run ./example</automated>
  </verify>
  <done>
Program exits 0 with deterministic output containing, in order: the F2 section header,
f2A Access/Gmem offset lines, the f2A Eval value (a multiple of 4 in [0,128)), the
informational SmemOptimize error line, the f2B SmemOptimize offset line, the sgc section
header, sgcA Access/Gmem/Smem offset lines, the sgcA Eval value (a multiple of 4 in
[0,1792)), a multi-line indented ast.Dump block, and the sgcB GmemOptimize offset line
with the rewrite note. Full gate (build, vet, test, gofmt on example/, run) green;
`go run ./example` output identical on two consecutive runs.
  </done>
</task>

<task type="auto">
  <name>Task 2: Extend README with integration-layer usage, example pointer, conventions</name>
  <files>README.md</files>
  <action>
Extend the EXISTING README (quick task 261121-om7 already wrote the layer-3 sections in
commit c23a066) — do not rewrite or duplicate the current usage snippet or the F₂
half. Five targeted edits, all claims must be verifiable against code or the example's
actual output (no aspirational language):

1. In `## Universal integration (pkg/ast + pkg/layout)`, near the existing code snippet,
   add the import block consumers need (`github.com/dianpeng/layoutalgebra`,
   `.../pkg/ast`, `.../pkg/layout`) and one sentence noting that the native algebra
   APIs live under `internal/` (in-module), so out-of-module consumers integrate
   against `pkg/layout` + `pkg/ast` only.
2. Add a `### Runnable example` subsection at the end of the Universal integration
   section (before the `---` separator): the command `go run ./example`, and a bullet
   list of what it demonstrates — the one-token algebra switch (`layout.F2` vs
   `layout.SubgroupChain`), all three stage methods per algebra with their Offset ASTs,
   an SmemOptimize conservative error handled informationally (not fatal), `ast.Eval`
   reference evaluation for both algebras, `ast.Dump` tree rendering, and the lane-first
   GMEM rewrite case. Match the example's actual behavior exactly.
3. In that subsection (or directly after it), document the Eval assignment naming
   convention: F₂ lowered ASTs consume per-bit variables (`w0`, `t1`, ...) because the
   algebra works at bit granularity, while sub-group-chain ASTs consume variable-level
   names (`w`, `t`) directly. State it as established behavior (it is — f2 bit naming
   is `v.Name+significance` in pkg/layout/f2.go).
4. In `## Verification` -> `Run it` code block, add a `go run ./example` line after the
   build/vet/test line.
5. In `## Project layout` tree, add an `example/` block directly after the `pkg/layout/`
   entries (`example/main.go` — runnable end-to-end example: both algebras, all stages).
   In `## Status`, append one sentence pointing at the runnable example — no new claims
   about unimplemented backends beyond what the section already says.

Then run the example once and re-read every sentence you added against its output;
delete or correct anything the run does not support.
  </action>
  <verify>
    <automated>cd /home/ps/Workspace/cuda/project/f2algebra && go build ./... && go vet ./... && go test ./... -count=1 && go run ./example > /dev/null && grep -c "go run ./example" README.md | grep -v '^0$' && grep -c "example/main.go" README.md | grep -v '^0$'</automated>
  </verify>
  <done>
README contains: the `go run ./example` run command (Verification section + Runnable
example subsection), an `example/` entry in the project-layout tree, the three
integration-layer import paths, the internal/-vs-pkg/ consumption note, and the
per-algebra Eval naming convention. No duplicated snippet, no aspirational claims; full
gate still green and example output unchanged from Task 1.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| none | The example reads no external input, opens no files, touches no network; README is documentation |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-rqf-01 | Tampering | example program output | accept | Pure local deterministic computation over library-internal data; no untrusted input crosses any boundary |
| T-rqf-02 | Supply chain | new package installs | mitigate | No new packages — Go stdlib only per project constraint; zero additions to go.mod |
| T-rqf-03 | Information disclosure | README claims | mitigate | Task 2 rule: every added claim must be verifiable against code or the example's actual run output |
</threat_model>

<verification>
Overall gate (must pass from repo root, in this order):
`go build ./... && go vet ./... && go test ./... -count=1`
`[ -z "$(gofmt -l example/)" ]` (scripts/format.sh does NOT cover example/ — it only checks internal/)
`go run ./example` — exit 0, deterministic output (identical on re-run), section
structure per Task 1 done criteria.
README accuracy: each added sentence checkable against source or the example run.
</verification>

<success_criteria>
- example/main.go exists, is package main, stdlib + project imports only, gofmt-clean
- The example runs both algebras through layout.New/NewSession/Access/GmemOptimize/
  SmemOptimize using exactly the four verified fixtures, prints String/Dump/Eval
  artifacts deterministically, and demonstrates graceful handling of the f2A
  SmemOptimize conservative error
- README extended (not rewritten) with the example pointer, import paths, internal/
  vs pkg/ note, and the F₂-bit vs sgc-variable Eval naming convention
- Full gate green; algebra.go, pkg/, and internal/ source untouched
- One atomic commit, conventional format (e.g. `feat(quick-261121-rqf): add runnable example and README integration-layer usage`)
</success_criteria>

<output>
Create `.planning/quick/261121-rqf-update-readme-with-integration-layer-inf/261121-rqf-SUMMARY.md` when done
</output>
