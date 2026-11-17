package f2

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
)

// Derive a permutation matrix A, s.t. A * lhs = rhs. Each rhs row claims the
// first unused equal lhs column; a column claimed by an earlier rhs row is
// skipped so duplicated coordinates can still map bijectively (WR-09).
func PermutationMatrix(
	lhs Vector,
	rhs Vector,
) (CoeffMat, error) {
	// rank mismatch is a caller error, not an invariant violation — report it
	// through the error return instead of panicking (WR-09a)
	if lhs.Rank() != rhs.Rank() {
		return CoeffMat{}, fmt.Errorf(
			"permutation matrix: rank mismatch %d vs %d",
			lhs.Rank(),
			rhs.Rank(),
		)
	}
	out := NewCoeffMatAllZero(lhs.Rank(), rhs.Rank())
	used := make([]bool, lhs.Rank()) // each lhs column may be claimed once (WR-09b)
	for i := 0; i < rhs.Rank(); i++ {
		r := rhs.Index(i) // corresponding RHS expression
		found := false
		for j := 0; j < lhs.Rank(); j++ {
			l := lhs.Index(j)
			if r.IsEqual(l) {
				if used[j] {
					// column claimed by an earlier rhs row — a later equal lhs column
					// may still make this a valid permutation, so keep scanning
					continue
				}
				used[j] = true
				out.On(i, j) // transfer (i element => j position)
				found = true
				break
			}
		}
		if !found {
			return CoeffMat{}, fmt.Errorf("incompatible input vectors, cannot generate permutation matrix")
		}
	}
	return out, nil
}

// Perform a simple Guassian Elimination, result in LUT (actually lower triangle). Mostly
// used to learn the rank of a coeff matrix. Notes the expression must be normalized via
// Fold, otherwise the parametrized version of the matrix may not be recognizable by the
// solver.
//
// Algorithm is standard
//
// 1) A cursor pair, always pointed at diagnoal element i'th row, cursor is (i, i)
// 2) The current pivot elements
//
// Notes, for now the elimination DOES NOT support none constant coefficient. Which
// should not be used in most of the cases. If we support it, we will need to support
// division in F2, since the limination scalar is parametrized by symbol with more
// operators need to be supported. This can be done, but not very useful for us.
//
// The elimiantion function will return some satellite information for later usage
// 1) rank
// 2) basis index, this is very useful for things like swizzle bank, which requires
//    a new basis to perform ADD(xor) for bank bit

type EliminationResult struct {
	Matrix     CoeffMat
	Rank       int
	BasisIndex []int // original input space
}

func GuassianEliminationConstMat(input CoeffMat) (EliminationResult, error) {
	constMat, err := input.ToConst()
	if err != nil {
		return EliminationResult{}, err
	}

	cursorI := 0 // row
	cursorJ := 0 // columns

	raw := constMat.Raw()
	m, n := constMat.Size()

	// tracking original index after permutation, used for basis index reconstruction
	permIdx := make([]int, 0, m)
	for i := 0; i < m; i++ {
		permIdx = append(permIdx, i)
	}

	swapRow := func(i, j int) {
		l := raw[i]
		raw[i] = raw[j]
		raw[j] = l

		index := permIdx[i]
		permIdx[i] = permIdx[j]
		permIdx[j] = index
	}

	subRawFrom := func(dst []F2, src []F2) {
		for idx, l := range dst {
			r := src[idx]
			dst[idx] = l.Sub(r)
		}
	}

	for cursorI < m && cursorJ < n {
		// (1) get the lhs row, which is used for elimination
		lhsRow := raw[cursorI]

		// (2) get the pivot element
		pivot := lhsRow[cursorJ]

		// (3) if pivot is zero, finding a valid one, and then perform swapping
		if pivot == Zero {
			for j := cursorI + 1; j < m; j++ {
				if candidatePivot := raw[j][cursorJ]; candidatePivot != Zero {
					pivot = candidatePivot // change pivot
					swapRow(cursorI, j)    // swap the row index
					lhsRow = raw[cursorI]  // pivot row now lives at cursorI after the swap
					break                  // symbol term will be treated as none zero, since we cannot know it
				}
			}
		}

		// (4) if cannot find another pivot, stay in pivot row, bump pivot column
		if pivot == Zero {
			cursorJ++
			continue
		}

		// (5) now, we can perform elimination accordingly. Notes, for us no scalar is needed
		//     under F2
		for j := cursorI + 1; j < m; j++ {
			if rhs := raw[j][cursorJ]; rhs {
				subRawFrom(raw[j], lhsRow)
			}
			// zero row, just skipped
		}

		cursorI++
		cursorJ++
	}

	// get the rank
	rank := cursorI
	basisIdx := permIdx[0:rank]

	// output
	return EliminationResult{
		Matrix:     NewCoeffMatFromConstMat(raw),
		Rank:       rank,
		BasisIndex: basisIdx,
	}, nil
}

func RankConstMat(input CoeffMat) (int, error) {
	if out, err := GuassianEliminationConstMat(input); err != nil {
		return -1, err
	} else {
		return out.Rank, nil
	}
}

// Derive a shift matrix for shifting the vector K bits away, assuming vector representing
// a binary bit pattern. (a1, a2, ..., am) << k => (ak, ak+1, ..., am). The shift is
// assumed to be unsigned integer shifts, ie bit left will be zeroed
func ShiftMatrix(size int, k int) CoeffMat {
	check.Check(k >= 0, fmt.Sprintf("shift matrix: negative k %d", k))
	if k >= size {
		return NewCoeffMatAllZero(size, size)
	}
	raw := make([][]F2, size)
	for i := 0; i < size; i++ {
		raw[i] = make([]F2, size)
	}
	row := 0
	for i := k; i < size; i++ {
		raw[row][i] = One
		row++
	}
	constMat := NewConstMat(raw)
	return constMat.ToCoeffMat()
}
