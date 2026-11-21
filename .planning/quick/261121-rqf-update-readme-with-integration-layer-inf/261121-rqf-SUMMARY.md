---
phase: quick-261121-rqf
plan: 01
subsystem: universal-integration
tags: [example, readme, docs, integration-layer, eval-convention]
requires:
  - pkg/layout Engine/Session/Mapping facade (F2 + SubgroupChain backends)
  - pkg/ast String/Dump/Eval reference artifacts
  - the four verified fixtures from the pkg/layout test suite
provides:
  - example/main.go — runnable end-to-end tour of the facade on both algebras
  - README integration-layer usage docs (import paths, internal/ note, example
    pointer, per-algebra ast.Eval naming convention)
affects:
  - README.md (extended only — five targeted edits, no rewrites)
tech-stack:
  added: []
  patterns:
    - deterministic example output (fixed assignments, String/Dump/Eval only)
    - expected stage errors printed informationally (conservative contract)
key-files:
  created:
    - example/main.go
  modified:
    - README.md
decisions:
  - expandBits in the example recomputes bit counts inline ((1<<j) < Bound) since
    pkg/layout's log2Checked is unexported — same convention as layout_test.go:67
  - ast.Dump already terminates every line with \n, so the example prints it with
    fmt.Print (not Println) to avoid a trailing blank line
metrics:
  duration: ~3 min
  completed: 2026-11-21
  tasks: 2
  files-created: 1
  files-modified: 1
  new-tests: 0 (example program; gated by build/vet/test/gofmt/run determinism)
---

# Phase quick-261121-rqf Plan 01: README Refresh + Example Program Summary

**One-liner:** Runnable `example/main.go` touring both algebras through the
`pkg/layout` facade (all three stages, String/Dump/Eval artifacts, f2A SMEM
conservative error handled informationally, sgcB lane-first GMEM rewrite) plus
five targeted README extensions documenting the integration layer — every claim
verified against the example's actual output.

## What Was Built

### Task 1 — example/main.go (commit f066df9)

- `package main`, 200 lines, stdlib (`fmt`, `os`) + the three project imports
  (`layoutalgebra`, `pkg/ast`, `pkg/layout`). `main` → `f2Demo()` then
  `sgcDemo()`; helpers `expandBits` (variable-level → per-bit assignment,
  same convention as pkg/layout/layout_test.go:67) and `fatalf` (unexpected
  failures only).
- Four fixtures lifted EXACTLY from the pkg/layout test suite (guardrail held —
  no swaps): f2A [4,8] d4 (Access OK, Gmem OK, Smem errors), f2B [4,32] d4
  (Smem saturates cleanly), sgcA [7,16,4] d4 lane-last (all stages OK),
  sgcB [2,96] d4 lane-first (Gmem draw path rewrites the mapping).
- Every predicted stage outcome held: f2A SmemOptimize returned the native
  no-free-degree error and was printed informationally with execution
  continuing; all other marked-success stages succeeded. No guardrail trips.
- Output structure (verified): F2 header → problem lines → Access/Gmem offset
  lines → Eval 84 (multiple of 4 in [0,128)) → informational Smem error →
  f2B Smem offset → sgc header → sgcA Access/Gmem/Smem offsets → Eval 792
  (multiple of 4 in [0,1792)) → multi-line ast.Dump block → sgcB Gmem offset
  + rewrite note.
- Determinism: two consecutive `go run ./example` invocations byte-identical
  (diff empty), exit 0 both times.

### Task 2 — README extension (commit 2943a42)

Five targeted edits, extend-only (nothing from 261121-om7's sections rewritten
or duplicated):

1. Import block (`layoutalgebra` + `pkg/ast` + `pkg/layout`) and the
   internal/-is-in-module consumption note inserted right before the existing
   usage snippet.
2. `### Runnable example` subsection before the `---` separator: the
   `go run ./example` command plus six bullets matching the program's actual
   behavior (one-token switch, three stages per algebra, informational
   conservative error on 4x8 then success on 4x32, Eval for both algebras,
   Dump tree, lane-first GMEM rewrite).
3. Eval naming convention paragraph inside that subsection: F₂ per-bit
   variables (`v`+`j`, e.g. `w0`, `t1` — pkg/layout/f2.go:19) needing bit
   expansion vs sgc variable-level names (`w`, `t`) directly.
4. `go run ./example` line added to the Verification Run-it block.
5. `example/` block in the project-layout tree (after pkg/layout/ entries) and
   a Status sentence pointing at the runnable example.

Claim audit: after the edits the example was re-run and every added sentence
checked against its output; nothing needed correcting or deleting. The sgcB
rewrite bullet is additionally machine-checked by
`TestSGCArtifactsImmutableAcrossOptimize` (pkg/layout/subgroupchain_test.go:331).

## Verification Results

- `go build ./... && go vet ./... && go test ./... -count=1` — green (whole
  repo, example/ included in the module build).
- `[ -z "$(gofmt -l example/)" ]` — clean (scripts/format.sh does not cover
  example/, so it was checked directly, both tasks).
- `go run ./example` — exit 0, byte-identical across three runs (two in Task 1,
  one in Task 2 confirming README edits changed nothing).
- README greps: `go run ./example` x4 (subsection, Verification, tree, Status),
  `example/main.go` x1, all three import paths present.
- `git diff 57b9b31 --stat -- algebra.go internal/ pkg/` — EMPTY; add-only
  discipline held (only example/ and README.md touched).

## Deviations from Plan

None — plan executed exactly as written. One environmental note: the plan's
`<verify>` blocks say to run gates from the main repo checkout; they were run
from the executor worktree instead (worktree isolation requirement), which is
the same source tree at the same base commit.

## Known Stubs

None. The example wires real library calls end to end; README claims are all
backed by code or the example run.

## Threat Flags

None. T-rqf-02 held (zero go.mod changes); T-rqf-03 held (claim audit above);
T-rqf-01 accepted as pure local deterministic computation.

## Self-Check: PASSED

- Files: example/main.go and README.md verified present on disk.
- Commits: f066df9 (feat, example) and 2943a42 (docs, README) verified in
  `git log` on branch worktree-agent-a3a3a96a3f252d0e2.
