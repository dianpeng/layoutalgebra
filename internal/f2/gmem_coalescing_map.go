package f2

import (
	"fmt"

	"github.com/dianpeng/f2algebra/internal/check"
)

// --------------------------------------------------------------------------------------
//
// GMEM coalescing map
//
// Integration with the compiler optimizer:
//
//  1) GMEM path: hardware coords (user-provided) -> GmemCoalescingMap (matrix M)
//     -> StrideMap -> final offset vector -> code generation
//  2) SMEM path: hardware coords -> MapAccess -> StrideMap -> SwizzleMap
//     -> final offset vector -> code generation (same emitter)
//
// The coalescing objective is evaluated on the LANE-RESTRICTED offset matrix:
// warp/block bits are fixed per warp instruction, so only the lane bits vary.
// For a warp of m lanes with dtype size 2^d bytes, using segment boundaries
// {Sector, Line} (NVIDIA default {5, 7}: 32B sectors, 128B lines):
//
//	lineRank   = rank( offset rows with byte weight >= 2^Line )   (distinct cache lines)
//	sectorRank = rank( offset rows with byte weight >= 2^Sector ) (distinct sectors)
//
// subject to the volume floors sectorRank >= m+d-Sector, lineRank >= max(0,
// m+d-Line). Perfect coalescing = both floors met. Since vectors are MSB-
// first, the low byte weights live at the END of the offset vector; the
// greedy placement — lane bits into the lowest logical positions — is
// provably optimal for this lexicographic objective, which is what
// GmemCoalescingMap emits.
//
// --------------------------------------------------------------------------------------

// Substitute returns e with every variable named in subst replaced by the
// corresponding expression, re-folded to canonical form. Variables absent
// from subst are left untouched. This is the partial-evaluation primitive:
// fixing warp bits to zero yields the lane-restricted matrix used by the
// coalescing analysis (and concrete M/N values at code-generation time).
func Substitute(e Expr, subst map[string]Expr) Expr {
	if len(subst) == 0 {
		return e
	}
	var sub func(x Expr) Expr
	sub = func(x Expr) Expr {
		switch node := x.(type) {
		case *NodeVariable:
			if r, ok := subst[node.Name()]; ok {
				return r
			}
			return x
		case *NodeSymbol:
			// symbols substitute too — this is the codegen-time
			// concretization path for static parameters (M, N, ...)
			if r, ok := subst[node.Name()]; ok {
				return r
			}
			return x
		case *NodeBinary:
			return Fold(NewBinary(node.Op(), sub(node.L()), sub(node.R())))
		default:
			return x
		}
	}
	return sub(e)
}

// SubstituteVector applies Substitute to every element of v.
func SubstituteVector(v Vector, subst map[string]Expr) Vector {
	out := make([]Expr, 0, v.Size())
	for i := 0; i < v.Size(); i++ {
		out = append(out, Substitute(v.Index(i), subst))
	}
	return NewVector(out)
}

// --------------------------------------------------------------------------------------
//
// Coalescing score — the objective/certification layer
//
// --------------------------------------------------------------------------------------

// CoalesceScore reports the lane-restricted coalescing objective of a
// byte-offset vector: how many distinct cache lines and sectors one warp
// instruction touches.
type CoalesceScore struct {
	LineRank   int // distinct lines = 2^LineRank
	SectorRank int // distinct sectors = 2^SectorRank (the transaction cost)
}

// Segment declares the GMEM segment boundaries for the coalescing analysis,
// mirroring Bank for SMEM. Sector is log2 of the sector size in bytes (the
// memory-transaction granularity); Line is log2 of the cache line size.
// Line must be strictly greater than Sector.
type Segment struct {
	Sector int
	Line   int
}

// GmemSegmentNvidia is the default Segment for NVIDIA GPUs — 32B sectors
// (2^5) and 128B lines (2^7), the architecture-fixed {5, 7} bit boundaries.
// Other architectures can declare their own via Segment{...}.
var GmemSegmentNvidia = Segment{Sector: 5, Line: 7}

func (s Segment) validate() error {
	if s.Sector < 0 || s.Line < 0 {
		return fmt.Errorf("Segment: negative boundary: %+v", s)
	}
	if s.Line <= s.Sector {
		return fmt.Errorf("Segment: Line (%d) must be strictly greater than Sector (%d)", s.Line, s.Sector)
	}
	return nil
}

// ScoreCoalescing evaluates the objective with the NVIDIA default segments
// (32B sectors, 128B lines).
func ScoreCoalescing(offset Vector, laneNames []string) (CoalesceScore, error) {
	return ScoreCoalescingWith(offset, laneNames, GmemSegmentNvidia)
}

// ScoreCoalescingWith evaluates the objective on a byte-offset vector with
// explicit segment boundaries. Non-lane variables are substituted with zero
// (the lane restriction: warp bits are constant within a warp), then the
// coefficient matrix over the lane basis is built and the line/sector row
// slices ranked exactly.
//
// Vectors are MSB-first: byte weight 2^k sits at index (size-1)-k, so the
// sector rows are indices [0, size-Segment.Sector) and the line rows are
// [0, size-Segment.Line).
func ScoreCoalescingWith(offset Vector, laneNames []string, seg Segment) (CoalesceScore, error) {
	if err := seg.validate(); err != nil {
		return CoalesceScore{}, err
	}
	check.Check(len(laneNames) > 0, "ScoreCoalescingWith: empty lane basis")

	// lane restriction: zero out every non-lane variable
	subst := map[string]Expr{}
	for _, name := range offset.BasisName() {
		lane := false
		for _, ln := range laneNames {
			if name == ln {
				lane = true
				break
			}
		}
		if !lane {
			subst[name] = NewConstZero()
		}
	}
	restricted := SubstituteVector(offset, subst)

	cm, err := restricted.CoeffMat(laneNames)
	if err != nil {
		return CoalesceScore{}, fmt.Errorf("ScoreCoalescing: %w", err)
	}

	rankOfRows := func(end int) (int, error) {
		m, n := cm.Size()
		if end <= 0 {
			return 0, nil
		}
		if end > m {
			end = m
		}
		rows := make([][]Expr, end)
		for i := 0; i < end; i++ {
			row := make([]Expr, n)
			for j := 0; j < n; j++ {
				row[j] = cm.Index(i, j)
			}
			rows[i] = row
		}
		res, err := GuassianEliminationConstMat(NewCoeffMat(rows))
		if err != nil {
			return 0, err
		}
		return res.Rank, nil
	}

	size := offset.Size()
	sectorRank, err := rankOfRows(size - seg.Sector)
	if err != nil {
		return CoalesceScore{}, err
	}
	lineRank, err := rankOfRows(size - seg.Line)
	if err != nil {
		return CoalesceScore{}, err
	}
	return CoalesceScore{LineRank: lineRank, SectorRank: sectorRank}, nil
}

// --------------------------------------------------------------------------------------
//
// The coalescing map generator — derives M
//
// --------------------------------------------------------------------------------------

// GmemAxis declares one logical dimension of the tile coordinate space:
// Bits is log2 of the axis size. Axes are ordered MSB-first — the last axis
// is the innermost (its bits occupy the lowest logical positions).
type GmemAxis struct {
	Name string
	Bits int
}

// CoalescePlan carries the generated transformation matrix plus the layout
// report used for integration decisions.
type CoalescePlan struct {
	Matrix CoeffMat // M: hw coords -> data logical coords, bijective

	Innermost     string // axis whose bits occupy the lowest logical positions
	Span          int    // innermost axis span in byte bits: Bits + dtypeSize
	LaneBitsFit   int    // lane bits absorbed by the innermost axis span
	LaneBitsSpill int    // lane bits spilling past the innermost axis
	Perfect       bool   // all lane bits inside the innermost span
}

// GmemCoalescingMap derives the bijective transformation M from hardware
// coordinates to data logical coordinates, optimized for GMEM coalescing.
//
// hw is the hardware coordinate vector, MSB-first; by CUDA convention the
// LAST laneBits entries are the warp lane bits (threadIdx.x is the
// fastest-varying dimension of the linear thread index, hence the lowest
// bits). axes declares the logical tile coordinate space, MSB-first, with
// total bit width equal to hw.Size(). dtypeSize is log2 of the dtype size in
// bytes.
//
// Placement is the provably optimal greedy: lane bits occupy the lowest
// logical positions (byte positions [dtypeSize, dtypeSize+laneBits) after
// the stride shift — contiguous, inside one 128B line whenever
// laneBits+dtypeSize <= 7), non-lane bits fill the leading positions
// preserving order. The returned Matrix is bijective by construction (a
// permutation matrix).
func GmemCoalescingMap(hw Vector, axes []GmemAxis, laneBits, dtypeSize int) (*CoalescePlan, error) {
	n := hw.Size()
	if len(axes) == 0 {
		return nil, fmt.Errorf("GmemCoalescingMap: no axes declared")
	}
	total := 0
	for _, a := range axes {
		if a.Bits <= 0 {
			return nil, fmt.Errorf("GmemCoalescingMap: axis %q has non-positive bits %d", a.Name, a.Bits)
		}
		total += a.Bits
	}
	if total != n {
		return nil, fmt.Errorf("GmemCoalescingMap: axes total %d bits != hw vector size %d", total, n)
	}
	if laneBits <= 0 || laneBits > n {
		return nil, fmt.Errorf("GmemCoalescingMap: laneBits %d out of range for hw size %d", laneBits, n)
	}
	if dtypeSize < 0 {
		return nil, fmt.Errorf("GmemCoalescingMap: negative dtypeSize %d", dtypeSize)
	}

	// M: trailing hw bits (lane bits) map to trailing data positions, leading
	// hw bits to leading positions, order preserved in both groups — the
	// identity assignment, which is exactly the optimal greedy under the
	// trailing-lane convention.
	m := NewCoeffMatAllZero(n, n)
	for i := 0; i < n; i++ {
		m.On(i, i)
	}

	inner := axes[len(axes)-1]
	span := inner.Bits + dtypeSize
	fit := laneBits
	if fit > span {
		fit = span
	}
	spill := laneBits - fit

	return &CoalescePlan{
		Matrix:        m,
		Innermost:     inner.Name,
		Span:          span,
		LaneBitsFit:   fit,
		LaneBitsSpill: spill,
		Perfect:       spill == 0,
	}, nil
}
