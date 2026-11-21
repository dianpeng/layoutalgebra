package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
)

// --------------------------------------------------------------------------------------
//
// Last-dimension analysis — universal component
//
// Every memory-hierarchy pass reasons about the last dimension: its window
// (low-weight flattened tail) carries the unit strides, so both the GMEM
// coalescing objective and the SMEM bank objective are defined there. The
// analysis flattens the (possibly nested) last dimension into its
// mixed-radix piece sequence and classifies the pieces by variable family.
//
// --------------------------------------------------------------------------------------

// LastDimAnalysis is the classified piece sequence of a layout's last
// dimension.
type LastDimAnalysis struct {
	// Pieces is the flattened piece sequence in flatten order (left = high
	// weights, right = low weights; the tail carries the unit strides).
	Pieces []Coord

	// Rest are the pieces with no involvement of the family; their combined
	// bound is RestSize (E).
	Rest     []Coord
	RestSize int

	// Relevant are the pure-family pieces; their combined bound is
	// RelevantSize (R').
	Relevant     []Coord
	RelevantSize int

	// Suffix is the product of the trailing pure-relevant pieces: the
	// contiguous family coverage at the low-weight tail.
	Suffix int
}

// AnalyzeLastDim flattens and classifies the last dimension by family.
func AnalyzeLastDim(layout *Layout, family string) (*LastDimAnalysis, error) {
	lastIdx := layout.Coordinate.Size() - 1
	pieces := []Coord{}
	if err := lastDimPieces(layout.Coordinate.Index(lastIdx), &pieces); err != nil {
		return nil, err
	}
	return classifyPieces(pieces, family)
}

// FamilyFreedom returns the product of the upper bounds of every DISTINCT
// Variable of the given family anywhere in the layout's coordinate tree,
// recursing into nested dimensions. Notes multiple digits can source the
// same Variable node (the coordinate tree is a DAG over its Variables), so
// each variable is counted once, by identity. An absent family yields the
// empty product, 1.
func FamilyFreedom(layout *Layout, family string) int {
	out := 1
	seen := make(map[*Variable]bool)
	count := func(e Expr) {
		Visit(
			e,
			func(Expr) bool { return true },
			func(e Expr) error {
				if varNode, ok := e.(*Variable); ok && varNode.Family() == family &&
					!seen[varNode] {
					seen[varNode] = true
					out *= int(varNode.UpperBound())
				}
				return nil
			},
		)
	}
	var walk func(h *HTuple)
	walk = func(h *HTuple) {
		for idx := 0; idx < h.Size(); idx++ {
			v := h.Index(idx)
			switch {
			case v.IsLeaf():
				count(v.Leaf())
			case v.IsHTuple():
				walk(v.HTuple())
			case v.IsTuple():
				for _, e := range v.Tuple().vec {
					count(e)
				}
			}
		}
	}
	walk(&layout.Coordinate)
	return out
}

// classifyPieces splits the flattened pieces into non-relevant / pure-family
// sides and computes the trailing family suffix.
func classifyPieces(pieces []Coord, family string) (*LastDimAnalysis, error) {
	out := &LastDimAnalysis{Pieces: pieces, RestSize: 1, RelevantSize: 1, Suffix: 1}

	// suffix: product of the trailing pure-family pieces
	idx := len(pieces)
	for idx > 0 {
		info := pieces[idx-1].CoordInfo()
		if !info.IsOnlyOccupiedByFamily(family) {
			break
		}
		idx--
	}
	out.Suffix = 1
	for _, p := range pieces[idx:] {
		out.Suffix *= int(p.UpperBound())
	}

	for _, p := range pieces {
		info := p.CoordInfo()
		has := info.HasFamily(family)
		only := info.IsOnlyOccupiedByFamily(family)
		switch {
		case only:
			out.Relevant = append(out.Relevant, p)
			out.RelevantSize *= int(p.UpperBound())
		case !has:
			out.Rest = append(out.Rest, p)
			out.RestSize *= int(p.UpperBound())
		default:
			return nil, fmt.Errorf(
				"last dimension contains an opaque mixed piece (bound %d) that cannot be split",
				int(p.UpperBound()))
		}
	}
	return out, nil
}

// lastDimPieces flattens the last dimension's coordinate value into its
// piece sequence in flatten order, running coordinate analysis on every leaf
// so each piece's CoordInfo is populated.
func lastDimPieces(v *HTupleValue, out *[]Coord) error {
	switch {
	case v.IsLeaf():
		return expandPieces(v.Leaf(), out)
	case v.IsHTuple():
		h := v.HTuple()
		for idx := 0; idx < h.Size(); idx++ {
			if err := lastDimPieces(h.Index(idx), out); err != nil {
				return err
			}
		}
		return nil
	case v.IsTuple():
		for _, e := range v.Tuple().vec {
			if err := expandPieces(e, out); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported last dimension structure")
	}
}

// expandPieces flattens a coord expression into its mixed-radix piece
// sequence:
//
//   - a Stride expands into its operand pieces, recursively
//   - a Coordinate sourcing a Variable is an opaque (pure) leaf piece
//   - a Coordinate sourcing a Stride is a DIGIT of that Stride: in mixed
//     radix, (src % radix) / stride extracts exactly the contiguous run of
//     source pieces whose weights lie in [stride, radix), so it expands into
//     that run — this is what the nested-dim sub-access produces
//   - a Coordinate sourcing a Coordinate flattens algebraically:
//     ((v % rc)/sc % r)/s == (v % min(r*sc, rc)) / (s*sc)
func expandPieces(e Expr, out *[]Coord) error {
	if !IsCoord(e) {
		return fmt.Errorf("last dimension contains a non-coord expression %T", e)
	}
	if _, err := AnalyzeCoord(e); err != nil {
		return err
	}

	switch n := e.(type) {
	case *Stride:
		for _, c := range n.Coord() {
			if err := expandPieces(c, out); err != nil {
				return err
			}
		}
		return nil

	case *Coordinate:
		// flatten chained Coordinates into an effective (radix, stride) over
		// the ultimate source
		radix, stride := n.Radix(), n.Stride()
		src := n.Source()
		for {
			inner, ok := src.(*Coordinate)
			if !ok {
				break
			}
			innerRadix, innerStride := inner.Radix(), inner.Stride()
			if radix*innerStride > innerRadix {
				radix = innerRadix
			} else {
				radix = radix * innerStride
			}
			stride *= innerStride
			src = inner.Source()
		}

		switch source := src.(type) {
		case *Variable:
			*out = append(*out, n)
			return nil

		case *Stride:
			// decompose the digit into its piece run: pieces whose weights
			// (suffix products of later bounds) lie in [stride, radix). The
			// decomposition is only possible when the digit's (radix, stride)
			// is piece-ALIGNED (both are suffix products of the piece
			// bounds); a misaligned digit crosses piece boundaries and stays
			// an atomic piece — its own analysis still provides the family
			// and bound needed by the suffix check and the reorder.
			weight := 1
			bounds := make([]int, len(source.Coord()))
			for i := len(source.Coord()) - 1; i >= 0; i-- {
				c := source.Coord()[i].(Coord)
				bounds[i] = int(c.UpperBound())
			}
			run := []Expr{}
			runProduct := 1
			for i := len(bounds) - 1; i >= 0; i-- {
				if stride <= weight && weight < radix {
					run = append([]Expr{source.Coord()[i]}, run...)
					runProduct *= bounds[i]
				}
				weight *= bounds[i]
			}
			if runProduct != int(radix/stride) {
				// misaligned digit: atomic piece
				*out = append(*out, n)
				return nil
			}
			for _, piece := range run {
				if err := expandPieces(piece, out); err != nil {
					return err
				}
			}
			return nil

		default:
			return fmt.Errorf(
				"last dimension contains an unsupported coordinate source %T", src)
		}

	default:
		return fmt.Errorf("last dimension contains an unsupported coord %T", e)
	}
}

// checkDimExtent asserts the classified pieces cover the last dim's extent —
// a sanity invariant of the disjoint partition.
func checkDimExtent(layout *Layout, an *LastDimAnalysis) {
	rank := layout.Shape.Last().Rank()
	check.Check(
		an.RestSize*an.RelevantSize == rank,
		fmt.Sprintf("classified pieces %d x %d != last dim rank %d",
			an.RestSize, an.RelevantSize, rank),
	)
}
