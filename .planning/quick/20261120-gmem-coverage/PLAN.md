---
type: quick
date: 2026-11-20
---

# GMEM coverage proposal implementation (fixes 1-5)

User approved the proposal; Claude implements, user reviews after.

## Implemented

1. familyFreedom recursion (V undercount bug): nested dims counted; leaf /
   tuple / htuple walk.
2. Suffix semantics: (2a) assembly order leftover ++ arrivals ++ relevant;
   (2b) fast path = trailing pure-relevant product (suffix) >= W, not bare
   product; (2c) reorder-only path when R' >= W but misplaced (stable
   partition + remap, no draw).
3. Divisor-search draw: achievable occupancies R'*draw with draw | E; tries
   each feasible draw >= ceil(W/R') ascending until the donor GCD chain
   realizes one; classified errors (not representable / not extractable /
   opaque mixed).
4. Nested donors: donor walk recurses into HTuple values; pending
   replacements carry (htuple, idx) so nested leaves are rewritten in place.
5. Nested last dim: lastDimPieces flattens the (possibly nested) last dim
   into its piece sequence — Strides expand, piece-ALIGNED digits decompose
   into their piece run (weights in [stride, radix)), chained coordinates
   flatten algebraically ((v%rc)/sc % r)/s == (v % min(r*sc,rc))/(s*sc),
   misaligned digits stay atomic; window + suffix on the flattened tail;
   remap already redistributes into nested shapes with lane at the tail.

## Numbers

- Property suite: 964 -> 1794 cases (nested shape variants added): 1423
  optimized, 371 conservative bailouts, all bijection-verified; sector-exact
  assertion on contiguous sweeps; suffix >= window on every success.
- Bailout classes: 190 not-extractable (GCD stalls), 181 misaligned-mixed
  (nested digits crossing piece boundaries), 0 not-representable at N<=48.
- Measured fixes: partial-window 16 -> 1 sector; fast-path-position 16 -> 2
  sectors (reorder path).

## Verification

go build / vet / test ./... green; -count=5 deterministic.
