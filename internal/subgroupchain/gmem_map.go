package subgroupchain

import "fmt"

// --------------------------------------------------------------------------------------
//
// GMEM coalescing pass — framework instantiation
//
// (1) The reason why we may have a need for GMEM optimization is due to **unorder of BoV**
//     User provides BoV in a invalid order which result in sub-optimal mapping
//
// (2) The GMEM optimization is simple permutation of varaibles, ie swapping in the relevant
//     variables from else where. The **relevant variables** are defined by **family** of
//     variable. Ie the upper integration must provide a configuration to descripe how the
//     GMEM optimization works, 1) # of bytes to be considered as coalesce lines;
//     2) family of variables that are relevant.
//
// (3) The objective is **suffix coverage**: the relevant family must occupy the
//     low-weight tail of the last dimension's flattened piece sequence with
//     combined bound >= W — coverage as a bare product anywhere in the dim is
//     not sector-optimal. GMEM has no transform stage: the global memory
//     layout is not ours to rearrange, so the only legal repairs are the
//     mapping-side reorder and draw.
//
// --------------------------------------------------------------------------------------

type GMemCoalesce struct {
	ByteSize       int    // # of bytes per coalesce line
	VariableFamily string // relevant variable family name
}

type gmemPass struct {
	config GMemCoalesce
}

func (self *gmemPass) Family() string { return self.config.VariableFamily }

func (self *gmemPass) ByteWindow(dtypeSize int) (int, error) {
	if self.config.ByteSize < dtypeSize || self.config.ByteSize%dtypeSize != 0 {
		return 0, fmt.Errorf(
			"coalesce line size %d is not a multiple of dtype size %d",
			self.config.ByteSize, dtypeSize)
	}
	return self.config.ByteSize, nil
}

// ObjectiveSatisfied: the family suffix covers the coalescing window.
func (self *gmemPass) ObjectiveSatisfied(ctx *Context) bool {
	return ctx.Analysis.Suffix >= ctx.Window
}

// GMEM has no mapping-independent transform stage.
func (self *gmemPass) TryMappingRepair(ctx *Context) (bool, error) { return false, nil }

// GMEM accepts every input: its objective is structural (suffix coverage)
// and needs no sweep variable.
func (self *gmemPass) Validate(ctx *Context) error { return nil }

// GMem optimizes the layout for global-memory coalescing: the relevant
// family is drawn into the low-weight tail of the last dimension until its
// suffix covers the coalescing window. Infeasible draws fail with a
// classified error and leave the layout untouched.
func GMem(env Env, config GMemCoalesce, dtypeSize int, layout *Layout) error {
	return Optimize(env, &gmemPass{config: config}, nil, dtypeSize, layout)
}
