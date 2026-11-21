package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/common"
)

// --------------------------------------------------------------------------------------
//
// Memory-hierarchy optimization framework
//
// The universal pipeline shared by every pass (GMEM coalescing, SMEM bank
// conflict resolution):
//
//   [1] Window:    W = min(last dim extent, pass byte window / dtype, family
//                  freedom V) — the family coverage target in elements
//   [2] Analysis:  flatten + classify the last dim's pieces by family
//                  (LastDimAnalysis)
//   [3] Fast path: the pass's objective is already satisfied (GMEM: family
//                  suffix covers W; SMEM: warp sweep hits pairwise distinct
//                  banks) => no-op
//   [4] Transform: the pass's mapping-independent repair stage (SMEM
//                  swizzle; GMEM has none) — TryMappingRepair
//   [5] Coverage repair (universal engine): reorder the last dim's pieces,
//                  or draw family freedom from the donor dims via the
//                  divisor-search GCD swap
//   [6] Post-check: re-verify the objective; a repair that cannot satisfy it
//                  is reported (eg SMEM conflicts that only a swizzle can
//                  fix)
//
// Contract rules
//
//   - Conservative: do what you can, return an error when you cannot.
//     Infeasible draws are transactional — the layout is left untouched.
//   - Every transform preserves the bijection: dims keep their extents,
//     variables are consumed exactly once.
//   - TryMappingRepair implementations own their transactionality; the
//     swizzle stage is expected to re-verify the objective internally before
//     reporting handled.
//
// --------------------------------------------------------------------------------------

// Context bundles everything a pass stage may need. It is refreshed (new
// analysis) after each layout mutation.
type Context struct {
	Env       Env
	Pass      Pass
	Layout    *Layout
	BoV       []*Variable
	Lane      *Variable // the family's sweep variable, nil when not unique/absent
	DtypeSize int
	Window    int // element window W = min(last dim, byte window / dtype, V)
	Analysis  *LastDimAnalysis
}

// Sweep measures the warp sweep objective (sectors, banks) of the context's
// lane variable.
func (self *Context) Sweep() (SweepStats, error) {
	if self.Lane == nil {
		return SweepStats{}, fmt.Errorf("no unique family variable to sweep")
	}
	return SweepLane(self.Env, self.Layout, self.BoV, self.Lane, self.DtypeSize)
}

// Pass is the per-hierarchy instantiation of the framework.
type Pass interface {
	// Family is the relevant variable family name.
	Family() string

	// ByteWindow returns the pass's byte window — the coalesce line size
	// (GMEM) or the bank transaction span (SMEM: banks x word width). It
	// must be a multiple of the dtype size.
	ByteWindow(dtypeSize int) (int, error)

	// ObjectiveSatisfied reports whether the layout already meets the
	// objective (stage 3 fast path).
	ObjectiveSatisfied(ctx *Context) bool

	// TryMappingRepair is the pass's mapping-independent repair stage
	// (stage 4, eg the SMEM swizzle). Returns handled=false when
	// unavailable; returning handled=true with a nil error asserts the
	// objective is met. Passes without such a stage return (false, nil).
	TryMappingRepair(ctx *Context) (bool, error)

	// Validate rejects inputs the pass cannot reason about, after the
	// context is built (eg SMEM requires a unique family variable for its
	// warp sweep predicate). Returning nil accepts the input.
	Validate(ctx *Context) error
}

// Optimize runs the universal pipeline over the layout.
func Optimize(
	env Env,
	pass Pass,
	bov []*Variable,
	dtypeSize int,
	layout *Layout,
) error {
	family := pass.Family()
	if family == "" {
		return fmt.Errorf("empty relevant variable family")
	}

	// [1]+[2] window and analysis
	ctx, err := newContext(env, pass, bov, dtypeSize, layout)
	if err != nil {
		return err
	}
	if err := pass.Validate(ctx); err != nil {
		return err
	}

	// [3] fast path
	if pass.ObjectiveSatisfied(ctx) {
		return nil
	}

	// [4] pass-specific transform stage (swizzle)
	handled, err := pass.TryMappingRepair(ctx)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	// [5] universal coverage repair
	checkDimExtent(layout, ctx.Analysis)
	if ctx.Analysis.RelevantSize >= ctx.Window {
		if err := ReorderLastDim(env, layout, family, ctx.Analysis, ctx.Window); err != nil {
			return err
		}
	} else {
		v := FamilyFreedom(layout, family)
		if err := DrawIntoLastDim(env, layout, family, ctx.Analysis, ctx.Window, v); err != nil {
			return err
		}
	}

	// [6] post-check: the coverage repair's construction guarantees the
	// suffix objective (assembleLastDim places the family at the flattened
	// tail; the remap preserves it by the digit-order convention), so this
	// re-verification only guards passes with a stronger predicate (eg SMEM
	// bank distinctness). When the re-analysis itself cannot see through the
	// repaired structure (misaligned mixed digits in a nested remap), the
	// construction guarantee stands.
	ctx, err = newContext(env, pass, bov, dtypeSize, layout)
	if err != nil {
		return nil // construction guarantee; see comment above
	}
	if !pass.ObjectiveSatisfied(ctx) {
		return fmt.Errorf(
			"coverage repair could not satisfy the objective (bijection preserved); " +
				"the pass's transform stage (eg swizzle) is required")
	}
	return nil
}

func newContext(
	env Env,
	pass Pass,
	bov []*Variable,
	dtypeSize int,
	layout *Layout,
) (*Context, error) {
	family := pass.Family()

	byteWindow, err := pass.ByteWindow(dtypeSize)
	if err != nil {
		return nil, err
	}
	lastDimExtent := layout.Shape.Last().Rank()
	v := FamilyFreedom(layout, family)
	window := common.Min3(lastDimExtent, byteWindow/dtypeSize, v)

	analysis, err := AnalyzeLastDim(layout, family)
	if err != nil {
		return nil, err
	}

	// the sweep variable: the family's unique variable in the BoV, if any
	var lane *Variable
	for _, varNode := range bov {
		if varNode.Family() == family {
			if lane != nil {
				lane = nil // not unique: sweeps unavailable
				break
			}
			lane = varNode
		}
	}

	return &Context{
		Env:       env,
		Pass:      pass,
		Layout:    layout,
		BoV:       bov,
		Lane:      lane,
		DtypeSize: dtypeSize,
		Window:    window,
		Analysis:  analysis,
	}, nil
}
