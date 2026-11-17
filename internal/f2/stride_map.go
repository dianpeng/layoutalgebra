package f2

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
)

// Stride mapping. Again, all expression is parametrized, working under symbolic space.
//
// (1) The input vector only contains bits to cover just the M + N bits
//     - It is used to represent (i, j); 0 <= i < M; 0 <= j < N
//     - The total byte space contains pow(2, M + N) * sizeof(dtype) bytes
//
// (2) The input vector, which already in coordinate space, if flatten back to integer
//     is already a a valid offset. But without the dtype size, hence we need to shift
//     the bit vector a little bit ensure we are in **byte space**
//
// --------------------------------------------------------------------------------------

func StrideMap(
	tileShape []int, // [M, N] means power of 2 digits, ie actual shape is pow(2, M) * pow(2, N)
	input Vector, // input parametrized vector
	dtypeSize int, // power of sizeof dtype, must be 0/1/2
) (Vector, error) {
	check.Check(len(tileShape) == input.Size())
	// dtypeSize is the power of the dtype size and must be 0/1/2 (WR-12)
	if dtypeSize < 0 || dtypeSize > 2 {
		return Vector{}, fmt.Errorf("stride map: dtypeSize must be 0..2, got %d", dtypeSize)
	}
	return input.ShiftL(dtypeSize, true), nil
}
