package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
	"github.com/dianpeng/layoutalgebra/internal/common"
)

// --------------------------------------------------------------------------------------
//
// GMEM optimization mapping
//
// (1) The reason why we may have a need for GMEM optimization is due to **unorder of BoV**
//     User provides BoV in a invalid order which result in sub-optimal mapping
//
// (2) The GMEM optimization is simple permutation of varaibles, ie swapping in the relevant
//     variables from else where. The **relevant variables** are defined by **family** of
//     variable. Ie the upper integration must provide a configuration to descripe how the
//     GMEM optimization works, 1) # of bytes to be considered as coalesce lines;
//     2) family of variables that are relevant. Notes 1) defines the relevant bit ranges
//     in offset space. Notes this optimization works in coordinate space, not in offset
//     space, unlike F2.
//
// (3) The GMEM optimization pass is able to optimize all cases
//     - 3.1) Relevant range is partially occupied
//     - 3.2) Relevant range is NOT occupied
//     - 3.3) Relevant range is fully occupied, ie optimal
//
// (4) Notes the GMEM optimization pass, assumes a simple layout for the tiling space
//
//     - 4.1) The provided tile is not necessarily to be a 1D layout, in fact, in most
//            case it is not, because most compiler do tiling
//
//     - 4.2) We only consider the last dimension, notes we assume the tile shape is not
//            permutatable, which is also the fact most likely. It means, a shape of
//            (M0, M1, ..., Mk-1), assuming to be flatten, the Mk-1 dimension is the one
//            we will consider. Assuming the byte range is Z, provided by configuration,
//            and the dtype size is D, we only consider min(Mk-1*D, Z) to be covered by
//            relevant variables. Other part will be ignored totally. Notes, in fact, the
//            dtype will be devided by both sides, ie will really be min(Mk-1, Z/D).
//            We assert on D | Z.
//
//     - 4.3) The window is additionally capped by the total freedom V of the relevant
//            family across the whole mapping: the last dimension can never be covered
//            by more relevant freedom than exists. Hence the optimization window is
//
//                W = min(Mk-1, Z/D, V)
//
//            and the pass tries to raise the relevant occupancy of the last dimension
//            from its current R' to exactly W by drawing W/R' freedom from the relevant
//            variables living in the other dimensions, displacing an equal amount of
//            non-relevant freedom back into those dimensions (swap out / swap in).
//
// --------------------------------------------------------------------------------------

type GMemCoalesce struct {
	ByteSize       int    // # of bytes per coalesce line
	VariableFamily string // relevant variable family name
}

type gmemMap struct{}

// windowSize computes W = min(lastDimExtent, lineSize, V) where V is the total
// freedom of the relevant family across the whole layout.
func (self *gmemMap) windowSize(
	dtypeSize int,
	config *GMemCoalesce,
	layout *Layout,
) (int, error) {
	if config.ByteSize < dtypeSize || config.ByteSize%dtypeSize != 0 {
		return 0, fmt.Errorf(
			"coalesce line size %d is not a multiple of dtype size %d",
			config.ByteSize, dtypeSize)
	}
	lineSize := config.ByteSize / dtypeSize
	lastDimExtent := layout.Shape.Last().Rank()
	freedom := self.familyFreedom(layout, config.VariableFamily)
	return common.Min3(lastDimExtent, lineSize, freedom), nil
}

// familyFreedom returns the product of the upper bounds of every Variable of
// the given family in the layout's coordinate tree. Each variable occurs
// exactly once (access bijection), so the product is the family's total
// freedom V. An absent family yields the empty product, 1.
func (self *gmemMap) familyFreedom(layout *Layout, family string) int {
	out := 1
	for idx := 0; idx < layout.Coordinate.Size(); idx++ {
		v := layout.Coordinate.Index(idx)
		if !v.IsLeaf() {
			continue // nested dims handled by recursion below via their leaves
		}
		Visit(
			v.Leaf(),
			func(Expr) bool { return true },
			func(e Expr) error {
				if varNode, ok := e.(*Variable); ok && varNode.Family() == family {
					out *= int(varNode.UpperBound())
				}
				return nil
			},
		)
	}
	return out
}

func (self *gmemMap) adjust(
	env *Env,
	va Coord, // access expression, in Coord, for last dimension of input tile
	relevant []Coord, // pure-relevant pieces of the last dimension (product R)
	layout *Layout,
	config *GMemCoalesce,
	window int,
) error {
	family := config.VariableFamily

	// split on variable family name provided
	restVarList, _ := PartiteCoord(va, family)

	// product of the non-relevant freedom living in the last dimension
	existedSize := 1
	for _, v := range restVarList {
		existedSize *= int(v.UpperBound())
	}

	// current relevant occupancy R and the amount P of freedom to draw
	relevantSize := 1
	for _, v := range relevant {
		relevantSize *= int(v.UpperBound())
	}
	if window%relevantSize != 0 {
		return fmt.Errorf(
			"window %d is not divisible by the current relevant occupancy %d",
			window, relevantSize)
	}
	draw := window / relevantSize // P
	check.Check(draw > 1)         // caller fast-path guarantees R < W

	// ------------------------------------------------------------------------------------
	// Core adjustment
	//
	// - Get a list of ready to be swapped out freedom dimension: restVarList
	// - Try to swap in freedom bounded by **variable family** for compensation
	//
	// - 2 lists may contain totally mis-aligned proposition
	//   - For example: lhs [2, 4] rhs: [4, 2]
	//
	// - To simplify the job
	//
	//   - Combine the swap out freedom dimension as a single one via Stride
	//   - Redistribute it to the swapped in the variable on the fly using Coordinate
	//
	// - The redistribution draws exactly `draw` freedom (W/R); the pool remainder
	//   stays in the last dimension
	//
	// - Lastly, after the redistribution, we end up with a new coord for the last dimension
	//   - Notes our layout allows **HTuple**, hence we need to run a map access again
	//
	// ------------------------------------------------------------------------------------

	// Part out of the freedom in relevant portion but not a relevant variable
	swapOutAll := env.Factory.StrideCoord(restVarList) // %swap_out_all
	swapOutAllSize := existedSize                      // size of all swap out variables
	AnalyzeRange(swapOutAll)

	// for swapped in, we need to perform replacement. Notes the per-dimension
	// replacements are collected during the draw loop and applied only after
	// the full draw succeeded, so an infeasible configuration leaves the
	// layout untouched.
	swapIn := []Coord{}
	remaining := draw
	type pendingReplacement struct {
		dim          int
		root         Expr
		replacements []Replacement
	}
	pending := []pendingReplacement{}

	for dim := 0; dim < layout.Coordinate.Size()-1; dim++ {
		currentValue := layout.Coordinate.Index(dim)
		if !currentValue.IsLeaf() {
			continue // nested dimension: conservative, cannot swap through it
		}
		currentAccess := currentValue.Leaf()
		if !IsCoord(currentAccess) {
			continue
		}
		dimCoord, err := AnalyzeCoord(currentAccess)
		if err != nil {
			return err
		}
		if !dimCoord.CoordInfo().HasFamily(family) {
			continue
		}
		_, candidativeList := PartiteCoord(dimCoord, family)

		swapInReplacement := []Replacement{}

		for _, cand := range candidativeList {
			if remaining == 1 {
				break
			}
			ub := int(cand.UpperBound())
			if ub <= 1 {
				continue
			}

			// This # of freedom we need to draw from the cand (ie candidative variable)
			portion := common.GCD(ub, remaining)
			if portion == 1 {
				continue // shares no factor with the remaining need, skip it
			}

			// (1) Split the candidative freedom with needed portion to be ready to swap in
			AnalyzeRange(cand.(Expr))
			lhs, rhs, err := env.Factory.Split(cand.(Expr), ub/portion, portion)
			if err != nil {
				return err
			}

			// (2) Record the swapped in portion, for later swap in
			swapIn = append(swapIn, rhs.(Coord))

			// (3) Split the swap out variable to obtain portion for replacement
			//
			// - 1) The old expression can be used up, ie lhs == nil
			// - 2) Otherwise, compose a combined variable, ie lhs != nil
			AnalyzeRange(swapOutAll)
			x, replacePortion, err := env.Factory.Split(
				swapOutAll,
				swapOutAllSize/portion,
				portion,
			)
			if err != nil {
				return err
			}
			swapOutAll = x            // update the swapOutAll
			swapOutAllSize /= portion // update the size
			remaining /= portion

			// (4) Figure out the swap in variaxble
			var rep Expr // replacement expression
			if lhs != nil {
				rep = env.Factory.StrideCoord(
					[]Coord{lhs.(Coord), replacePortion.(Coord)},
				)
			} else {
				rep = replacePortion
			}

			// (5) Record replacement
			swapInReplacement = append(swapInReplacement, Replacement{
				Old: cand.(Expr),
				New: rep,
			})
		}

		// record the per-dimension replacement, applied after the whole draw
		// succeeded (see the comment on `pending`)
		if len(swapInReplacement) > 0 {
			pending = append(pending, pendingReplacement{
				dim:          dim,
				root:         currentAccess,
				replacements: swapInReplacement,
			})
		}

		if remaining == 1 {
			break
		}
	}

	if remaining != 1 {
		return fmt.Errorf(
			"insufficient relevant freedom in the other dimensions: %d left to draw",
			remaining)
	}

	// all splits succeeded: commit the rewrites into the other dimensions
	for _, p := range pending {
		layout.Coordinate.SetLeaf(p.dim, Replace(p.root, p.replacements))
	}

	// Finally, run a access map again for the last dimension, since we will end up with
	// a new combined variables
	//
	//   new last dim = swapIn (draw) ++ relevant (R) ++ leftover pool (E/draw)
	//
	// whose product is draw * R * (E/draw) = E * R = the original last dim extent
	newPieces := append([]Coord{}, swapIn...)
	newPieces = append(newPieces, relevant...)
	if swapOutAllSize > 1 {
		check.Check(swapOutAll != nil)
		newPieces = append(newPieces, swapOutAll.(Coord))
	}
	newLastDimVariable := env.Factory.StrideCoord(newPieces)
	AnalyzeRange(newLastDimVariable)

	lastShape := layout.Shape.Last()
	lastIdx := layout.Coordinate.Size() - 1
	if lastShape.IsRecursive() {
		newHTuple, err := AccessWithCoord(
			*env,
			[]Coord{newLastDimVariable.(Coord)},
			lastShape.AsHTuple(),
		) // remap the last dimension of the input shape, this will create recursive mapping if needed
		if err != nil {
			return err
		}
		layout.Coordinate.SetHTuple(lastIdx, newHTuple)
	} else {
		flat, err := AccessWithCoord(
			*env,
			[]Coord{newLastDimVariable.(Coord)},
			NewConstHTuple([]Value{lastShape.Rank()}),
		)
		if err != nil {
			return err
		}
		check.Check(flat.Size() == 1)
		layout.Coordinate.SetLeaf(lastIdx, flat.Index(0).Leaf())
	}

	return nil
}

func (self *gmemMap) run(
	env *Env,
	config GMemCoalesce,
	dtypeSize int,
	layout *Layout,
) error {
	if config.VariableFamily == "" {
		return fmt.Errorf("empty relevant variable family")
	}

	// [1] get the corresponding maximized optimizable size
	window, err := self.windowSize(dtypeSize, &config, layout)
	if err != nil {
		return err
	}

	// [2] get the last dimension's access expression; it must be a flat coord
	lastIdx := layout.Coordinate.Size() - 1
	lastValue := layout.Coordinate.Index(lastIdx)
	if !lastValue.IsLeaf() {
		return fmt.Errorf("GMEM optimization requires a flat last dimension")
	}
	lastAcc := lastValue.Leaf()
	if !IsCoord(lastAcc) {
		return fmt.Errorf("GMEM optimization requires the last dimension to be a Coord")
	}

	// [3] Perform a variable analysis, ie propagate variable family name
	va, err := AnalyzeCoord(lastAcc)
	if err != nil {
		return err
	}

	// [4] Checking fast path, ie the relevant occupancy already covers the window
	restList, relevantList := PartiteCoord(va, config.VariableFamily)
	relevantSize := 1
	for _, v := range relevantList {
		relevantSize *= int(v.UpperBound())
	}
	if relevantSize >= window {
		return nil // already optimal, do nothing
	}
	_ = restList

	// [5] readjust
	return self.adjust(env, va, relevantList, layout, &config, window)
}

func GMem(env Env, config GMemCoalesce, dtypeSize int, layout *Layout) error {
	x := gmemMap{}
	return x.run(&env, config, dtypeSize, layout)
}
