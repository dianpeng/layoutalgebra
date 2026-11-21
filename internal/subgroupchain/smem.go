package subgroupchain

import (
	"fmt"
)

// --------------------------------------------------------------------------------------
//
// SMEM bank-conflict pass — framework instantiation
//
// Bank model (NVIDIA): bank = (byteOffset / 4) % 32 — 32 banks of 4B words.
// A warp's access is conflict-free iff its lane sweep hits pairwise
// distinct banks. The objective is WEAKER than GMEM's contiguity (a strided
// sweep can be bank-perfect but sector-poor), so the fast path is sweep
// distinctness, not suffix coverage.
//
// Pipeline (per the framework):
//
//   [3] fast path: sweep distinctness (bank conflict-free) => no-op
//   [4] swizzle:   the mapping-independent repair — the transvection
//                  (bankPart + c * laneHigh) mod X — provided via the
//                  TrySwizzle hook. Notes the shear changes the SMEM STORAGE
//                  arrangement as well as the access expression, which is
//                  exactly the freedom GMEM does not have. NOT PROVIDED by
//                  this library yet: the hook is the extension point.
//   [5] coverage repair (universal engine): draw the family into the last
//                  dim's tail until its suffix covers the bank window — a
//                  valid fallback when no swizzle is available, at the cost
//                  of changing which thread loads which element (the GMEM
//                  pattern of the copy into SMEM).
//   [6] post-check: distinctness re-verified; conflicts that survive the
//                  coverage repair are reported as requiring the swizzle
//                  stage.
//
// --------------------------------------------------------------------------------------

// SmemSwizzle is the stage-4 transform hook: given the context (window,
// analysis, sweep available via ctx), attempt the bank-conflict repair.
// Returns handled=false when the swizzle cannot apply; handled=true with a
// nil error asserts the objective is met and the layout remains a bijection.
// Implementations own their transactionality.
type SmemSwizzle func(ctx *Context) (bool, error)

type SmemCoalesce struct {
	VariableFamily string

	// BankBytes is the bank transaction span in bytes
	// (32 banks x 4B words = 128 by default).
	BankBytes int

	// TrySwizzle is the optional stage-4 hook (see SmemSwizzle).
	TrySwizzle SmemSwizzle
}

type smemPass struct {
	config SmemCoalesce
}

func (self *smemPass) Family() string { return self.config.VariableFamily }

func (self *smemPass) ByteWindow(dtypeSize int) (int, error) {
	bankBytes := self.config.BankBytes
	if bankBytes == 0 {
		bankBytes = SmemBankCount * SmemBankWidthBytes // 128B default
	}
	if bankBytes < dtypeSize || bankBytes%dtypeSize != 0 {
		return 0, fmt.Errorf(
			"bank span %d is not a multiple of dtype size %d",
			bankBytes, dtypeSize)
	}
	return bankBytes, nil
}

// ObjectiveSatisfied: a warp sweep over the family variable is bank
// conflict-free (distinct banks saturate). When no unique sweep variable
// exists, fall back to suffix coverage of the bank window — coverage of a
// contiguous window implies distinct banks.
func (self *smemPass) ObjectiveSatisfied(ctx *Context) bool {
	if ctx.Lane != nil {
		stats, err := ctx.Sweep()
		if err == nil {
			return stats.BankConflictFree()
		}
	}
	return ctx.Analysis.Suffix >= ctx.Window
}

// TryMappingRepair delegates to the swizzle hook when provided.
func (self *smemPass) TryMappingRepair(ctx *Context) (bool, error) {
	if self.config.TrySwizzle == nil {
		return false, nil
	}
	return self.config.TrySwizzle(ctx)
}

// Validate: the bank-distinctness objective is a WARP SWEEP predicate — it
// needs the family to identify exactly one concurrency variable (the lane).
// A multi-variable family has no single warp sweep to measure, and silently
// falling back to suffix coverage would claim success while the swept
// variable still strides into bank collisions, so it is rejected.
//
// Notes for a unique family variable, coverage implies conflict-freedom
// under the one-warp model (each 32-value window chunk of the sweep is
// contiguous over the bank range), so the coverage fallback is sound and
// the swizzle stage is only required for windows the factorization cannot
// represent (eg nested cut structures).
func (self *smemPass) Validate(ctx *Context) error {
	count := 0
	for _, v := range ctx.BoV {
		if v.Family() == self.config.VariableFamily {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf(
			"smem pass requires a unique family variable for the warp sweep; "+
				"family %q has %d variables",
			self.config.VariableFamily, count)
	}
	return nil
}

// Smem resolves shared-memory bank conflicts for the layout: fast-path when
// the warp sweep is already conflict-free, the caller's swizzle when it can
// apply, and the universal coverage draw as the fallback. The BoV is needed
// to identify the sweep variable.
func Smem(
	env Env,
	config SmemCoalesce,
	bov []*Variable,
	dtypeSize int,
	layout *Layout,
) error {
	return Optimize(env, &smemPass{config: config}, bov, dtypeSize, layout)
}

// SmemBankPredicate is a verification helper: it reports whether a layout's
// warp sweep is bank conflict-free. Exposed for tests and diagnostics.
func SmemBankPredicate(
	env Env,
	layout *Layout,
	bov []*Variable,
	lane *Variable,
	dtypeSize int,
) (bool, error) {
	stats, err := SweepLane(env, layout, bov, lane, dtypeSize)
	if err != nil {
		return false, err
	}
	return stats.BankConflictFree(), nil
}
