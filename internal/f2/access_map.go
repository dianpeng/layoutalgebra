package f2

// --------------------------------------------------------------------------------------
//
// Access mapping
//
// 1) Takes one hardware vector, parameterized with bit variable
// 2) Takes one tile vector, parameterized with bit variables
//
// --------------------------------------------------------------------------------------

// --------------------------------------------------------------------------------------
//
// [1] The simplest path, ie Permutation Matrix. This resolution is not by solving a
//     linear equation, instead, we directly generate it on the fly. However, the
//     requirements are: each vector's all dimensional parameterized expression must be
//     equal ie same set. Notes, same set, which means unordered. The order resolution
//     is done by the permutation matrix
//
// --------------------------------------------------------------------------------------

func MapAccess(
	hardwareCoord Vector,
	dataCoord Vector,
) (Vector, error) {
	// (1) Using permutation matrix
	permMatrix, err := PermutationMatrix(dataCoord, hardwareCoord)
	if err != nil {
		return Vector{}, err
	}

	// (2) Apply the permutation matrix. Ie we get a parametrized data coord vector from
	//     the hardware coordinator
	return permMatrix.Apply(hardwareCoord), nil
}
