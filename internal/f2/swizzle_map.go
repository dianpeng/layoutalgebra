package f2

import (
  "fmt"
  "strings"

  "github.com/dianpeng/f2algebra/internal/common"
  "github.com/dianpeng/f2algebra/internal/check"
)

// Swizzle bank analysis against the the F2 vector
//
// Mostly, we are assuming we are using CUDA. To make this procedure smooth, we allow
// a configurable configuration to be specified. Notes, this is not parametrized one
// but instead, just allowing user to setup the bank # calculation.
//
// In general, bank # calculation is modeled as: (skip_bits, range). For cuda this is
// (2, 5), which means skip the LSB 2 bits, and in range [2, 7), pinned down as the
// bank conflicts range.
var SwizzleErrorUnknownBasis = fmt.Errorf("swizzle error: basis is unknown to us")
var SwizzleErrorSaturated    = fmt.Errorf("swizzle error: optimization is saturated")
var SwizzleErrorNoFreeDegree = fmt.Errorf("swizzle error: no free degree")

type Bank struct {
  Skip int
  Range int
  VariablePrefix string // which variable prefix will be assumed to be induction variable
}

type swizzleVectorInfo struct {
  Rank int         // rank of the vector's coefficient matrix
  BasisIndex []int // basis index in its coefficient matrix
  Coeff CoeffMat   // coefficient matrix object
  U     CoeffMat   // upper triangle
}

type swizzler struct {}

// NameBelong reports whether name begins with the given variable prefix.
func NameBelong(name, prefix string) bool {
  return strings.HasPrefix(name, prefix)
}

// BelongVariableNameFamily reports whether any variable in the expression
// tree belongs to the given variable name prefix family.
func BelongVariableNameFamily(e Expr, prefix string) bool {
  found := false
  Visit(
    e,
    func(val Expr) bool {
      _, ok := val.(*NodeVariable)
      return ok
    },
    func(v Expr) error {
      if NameBelong(v.(*NodeVariable).Name(), prefix) {
        found = true
      }
      return nil
    },
  )
  return found
}

func (self *swizzler) vectorInfo(vec Vector) (swizzleVectorInfo, error) {
  if coeffMat, err := vec.CoeffMatAll(); err != nil {
    return swizzleVectorInfo{}, err
  } else if !coeffMat.IsConst() {
    return swizzleVectorInfo{}, SwizzleErrorUnknownBasis // SHOULD never happen
  } else {
    if result, err := GuassianEliminationConstMat(coeffMat); err != nil {
      return swizzleVectorInfo{}, err
    } else {
      return swizzleVectorInfo{
        Rank: result.Rank,
        BasisIndex: result.BasisIndex,
        Coeff: coeffMat,
        U: result.Matrix,
      }, nil
    }
  }
}

// --------------------------------------------------------------------------------------
//
// The algorithm for swizzle bank
//
// For a input vector, **after stride mapping**, it is broken down into
// a list of bit expression as a bit vector. Inside of it, it could result in some
// bit variables, that can contribute freedom for bank swizzle.
//
// Goal: derive a new Vector, s.t it avoids bank conflicts as much as possible
//  - Bank #: input_vector >> Bank.Skip and the last Bank.Range bits, denoted as bank bits
//  - bank variables: a series of bit variables that decide the parallism that contribute to bank conflicts
//    - On Nvidia, it is thread bits per warp, or lane bit variable
//      - lane[k] = (threadIdx.x >> k) & 1 (0 <= k < 5)
//    - It all represnted as variable in our parametrized expression tree
//
//  - The bank bits range in input vector forms a bank bit vector
//    - It has a rank based on bank variables defined, denoted as R
//  - If R == Bank.Range, which means fully saturated, ie no room for performance bump
//  - The can be optimized size for bank bits are Rbest = Bank.Range - R
//  - Otherwise, we start to search other basis **besides the bank rank bits**
//    - Denoted as free bit vector. Free bit vector MUST also based on lane bit variable as basis
//  - Denote rank of the free bit vector as Rfree
//  - The maximimu allowed swizzle dimenion, # of dimension can be optimized is min(Rfree, Rbest)
//  - For each such dimension, we map to a basis found in free vector, and swizzle the existed value against it
//
// The transformation is essentially a matraix M
//
// 1) For none involved input dimension, it is identity, ie M[i][i] = 1 for i'th dimension
// 2) For involved input dimension, assuming its bank bits index is k, free vector bit is q
//
//    M[k][q] = 1
//    M[k][k] = 1
//
//    add = xor
//
// 3) Notes, the M is always bijective, since its inverse(M) = M, due to xor twice sends the
//    value back under F2
//
// --------------------------------------------------------------------------------------

func (self *swizzler) Run(
  bank Bank, // range of banks
  input Vector,        // input vectors
) (Vector, error) {
  check.Check(bank.Skip >= 0)
  check.Check(bank.Range >= 1)

  // (1) obtain a slice of vector for bank bits analysis
  bankBits := input.Slice(bank.Skip, bank.Skip + bank.Range)
  bankRank := -1
  bankBasis := []int{} // bank's basis index
  bankNS:= []int{}     // bank's nullspace index

  // ensure the bank bits ONLY contain allowed variable as basis, otherwise, not supported
  for _, name := range bankBits.BasisName() {
    if !NameBelong(name, bank.VariablePrefix) {
      // contains basis which is not known to us
      return Vector{}, SwizzleErrorUnknownBasis
    }
  }

  // checking weather the bankBits is fully ranked, if so, we cannot do better by swizzle
  if result, err := self.vectorInfo(bankBits); err != nil {
    return Vector{}, err
  } else {
    bankRank = result.Rank
    bankBasis = result.BasisIndex // basis index
    for i := 0; i < bankBits.Size(); i++ {
      found := false
      for _, j := range bankBasis {
        if i == j {
          found = true
          break
        }
      }
      if !found {
        bankNS = append(bankNS, i)
      }
    }

    // 4 foundational linear aglebra theorm
    check.Check(len(bankNS) == bankBits.Rank() - len(bankBasis))
  }

  // checking bank rank against possible maximum permutation
  if bankRank == bank.Range {
    // Already satifisied the optimal situation, do nothing
    return Vector{}, SwizzleErrorSaturated
  }

  // (2) now, we need to finding out the dimension that CAN BE XORed in order to perform
  //     swizzle change in the iteration/permutation space. To address this issue, one
  //     simple way is to chase the bit range (bank.Range, ...) and hit a dimension with
  //     full variable name, (ie basis) that from the same variable family
  freeIdx := []int{}
  for i := bank.Range + 1; i < input.Size(); i++ {
    if dimension := input.Index(i); BelongVariableNameFamily(
      dimension,
      bank.VariablePrefix,
    ) {
      freeIdx = append(freeIdx, i)
    }
  }

  // (3) gather a new vector and check weather its rank matches or not
  freeVector := input.Gather(freeIdx)
  freeRank := -1
  freeBasisIdx := []int{}
  result, err := self.vectorInfo(freeVector)
  if err != nil {
    return Vector{}, err
  }
  if result.Rank == 0 {
    // We cannot find any free degree for swizzle bank permutation
    return Vector{}, SwizzleErrorNoFreeDegree
  }
  // Using freeIdx to convert it back to freeVector space
  for _, bIdx := range result.BasisIndex {
    freeBasisIdx = append(freeBasisIdx, freeIdx[bIdx])
  }
  freeRank = result.Rank

  // (4) now we know that free vector contains dimension CAN BE used for combination
  //     for swizzle bank.
  //
  //     Basically, for each basis, we can permute each one with an XOR
  readyToBeSwizzledDimensionSize := common.Select(
    len(bankNS) < freeRank,
    len(bankNS),
    freeRank,
  )

  // perform swizzel directly
  for idx, nsIdx := range bankNS {
    if idx >= readyToBeSwizzledDimensionSize {
      break
    }
    inputIdx := bank.Skip + nsIdx                // index in input vector space
    existedValue := input.Index(inputIdx)        // existed value, which does not change
    swizzleValue := NewBinary(                   // swizzle value
      Xor,
      existedValue,
      freeVector.Index(freeBasisIdx[idx]), // free basis expression
    )
    input.Set(inputIdx, swizzleValue) // replace with newly created swizzle value
  }
  return input, nil
}

func SwizzleMap(bank Bank, input Vector) (Vector, error) {
  sw := swizzler{}
  return sw.Run(bank, input)
}
