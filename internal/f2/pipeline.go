package f2

import "fmt"

// --------------------------------------------------------------------------------------
//
// Layout optimization pipelines — the two combination entry points the
// compiler optimizer drives:
//
//  1) GMEM:  hardware coords -> GmemCoalescingMap (M) -> StrideMap
//     -> final offset vector + certified coalescing score
//  2) SMEM:  hardware coords -> access map (MapAccess permutation) -> StrideMap
//     -> SwizzleMap -> final offset vector
//
// Both return a Vector: the bit pattern of the final offset space, which the
// code generator interprets into index arithmetic. The two pipelines produce
// the same kind of artifact, so one emitter serves both.
//
// --------------------------------------------------------------------------------------

// GmemResult is the output of the GMEM coalescing pipeline.
type GmemResult struct {
	Offset Vector        // final offset-space bit pattern for code generation
	Plan   *CoalescePlan // layout decision report (includes the matrix M)
	Score  CoalesceScore // certified coalescing objective on the final vector
}

// trailingLaneNames extracts the names of the last laneBits entries of hw —
// the lane-basis names under the CUDA trailing-lane convention.
func trailingLaneNames(hw Vector, laneBits int) ([]string, error) {
	n := hw.Size()
	out := make([]string, 0, laneBits)
	for k := 0; k < laneBits; k++ {
		v, ok := hw.Index(n - 1 - k).(*NodeVariable)
		if !ok {
			return nil, fmt.Errorf("lane bit at hw position %d is %v, not a bare variable", n-1-k, hw.Index(n-1-k))
		}
		out = append(out, v.Name())
	}
	return out, nil
}

// GmemOptimize runs the GMEM coalescing pipeline end to end: derives the
// bijective transformation M from hardware coordinates to data logical
// coordinates, applies it, shifts into byte space, and certifies the
// lane-restricted coalescing objective on the final offset vector.
func GmemOptimize(hw Vector, axes []GmemAxis, laneBits, dtypeSize int) (*GmemResult, error) {
	plan, err := GmemCoalescingMap(hw, axes, laneBits, dtypeSize)
	if err != nil {
		return nil, err
	}
	mapped := plan.Matrix.Apply(hw)
	strided, err := StrideMap(make([]int, mapped.Size()), mapped, dtypeSize)
	if err != nil {
		return nil, fmt.Errorf("GmemOptimize: stride map: %w", err)
	}
	laneNames, err := trailingLaneNames(hw, laneBits)
	if err != nil {
		return nil, err
	}
	score, err := ScoreCoalescing(strided, laneNames)
	if err != nil {
		return nil, fmt.Errorf("GmemOptimize: score: %w", err)
	}
	return &GmemResult{Offset: strided, Plan: plan, Score: score}, nil
}

// SmemResult is the output of the SMEM swizzle pipeline.
type SmemResult struct {
	Offset   Vector // final offset-space bit pattern for code generation
	Swizzled bool   // false = bank already saturated, nothing to optimize
}

// SmemOptimize runs the SMEM bank-conflict pipeline end to end: maps hardware
// coordinates to the logical data layout, shifts into byte space, and
// applies the XOR swizzle.
//
// dataCoord declares the logical data coordinate layout. Two modes:
//
//   - Same size as hw: treated as a permutation of the hardware coordinates
//     and routed through MapAccess (bijective by construction).
//   - Different size: used directly as the logical coordinate expressions
//     over the hw basis — the rectangular/injective patterns (multi-thread
//     per element, sub-tile access) that MapAccess cannot express.
//
// SwizzleErrorSaturated is not an error here: it means the bank slice is
// already fully ranked (conflict-free), and the strided layout is returned
// unchanged with Swizzled=false. Other failures (e.g. no free degree) are
// propagated per the conservative contract.
func SmemOptimize(hw Vector, dataCoord Vector, dtypeSize int, bank Bank) (*SmemResult, error) {
	var logical Vector
	if hw.Size() == dataCoord.Size() {
		mapped, err := MapAccess(hw, dataCoord)
		if err != nil {
			return nil, fmt.Errorf("SmemOptimize: access map: %w", err)
		}
		logical = mapped
	} else {
		logical = dataCoord
	}

	strided, err := StrideMap(make([]int, logical.Size()), logical, dtypeSize)
	if err != nil {
		return nil, fmt.Errorf("SmemOptimize: stride map: %w", err)
	}

	out, err := SwizzleMap(bank, strided)
	if err == SwizzleErrorSaturated {
		return &SmemResult{Offset: strided, Swizzled: false}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("SmemOptimize: swizzle: %w", err)
	}
	return &SmemResult{Offset: out, Swizzled: true}, nil
}
