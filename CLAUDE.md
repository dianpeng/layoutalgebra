<!-- GSD:project-start source:PROJECT.md -->
## Project

**F2 Algebra — CUDA Layout Symbolic Analysis**

A Go library for symbolic analysis of CUDA layout calculation, written as a basic F₂ algebra.
The user sets up an F₂ algebra interpreter parametrized by symbolic tile dimensions (M, N); the
library performs symbolic calculation to generate a mapping from **hardware coordinates**
(threadIdx.x/y/z bits) to a **final memory offset** in a tile. The result is a *vector* — one
F₂ expression per output offset bit, over the input coordinate bits. From that vector the user
generates code directly by interpreting it.

Claude's role here is **passive assistance**: the user writes the code and calls Claude in for
(1) finding bugs and logical issues, (2) completing code — fixing bugs and writing comprehensive
unit tests, (3) utility features (expression dumpers, matrix/vector printers, etc.).

**Core Value:** Given hardware coordinate bits, correctly and verifiably derive the symbolic bit-level mapping
to a tile offset as an F₂ vector the user can interpret into generated code.

### Constraints

- **Tech stack**: Go stdlib only for tests — no external test frameworks or assertion libraries.
- **Dependencies**: keep third-party imports near zero; this is a small self-contained algebra library.
- **Authorship model**: user writes the core code; Claude fixes bugs, writes tests, and adds utilities on request — avoid rewriting user code wholesale.
- **Correctness**: F₂ bit-level math is unforgiving; every bug fix and utility must come with unit tests.
<!-- GSD:project-end -->

<!-- GSD:stack-start source:STACK.md -->
## Technology Stack

Technology stack not yet documented. Will populate after codebase mapping or first phase.
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
