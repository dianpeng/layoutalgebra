package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/common"
)

// Mapping a coordinate back to a 1D codomain, interpreted as **byte oriented** offset
// Notes, for byte oriented offset, we also need to take dtype's size into consideration

type strideMap struct{}

func (self *strideMap) doStrideMap(env Env, input HTuple, tileSize ConstHTuple) (Expr, error) {
	if input.Size() != tileSize.Size() {
		return nil, fmt.Errorf(
			"coordinate rank %d does not match shape rank %d",
			input.Size(), tileSize.Size())
	}
	size := input.Size()
	strideOffs := make([]StrideOffset, 0, size)

	for idx := 0; idx < size; idx++ {
		rhsTuple := tileSize.Index(idx)
		lhsValue := input.Index(idx)
		if rhsTuple.IsRecursive() {
			if !lhsValue.IsRecursive() {
				return nil, fmt.Errorf(
					"structure mismatch at dim %d: shape is recursive but coordinate is a leaf",
					idx)
			}
			coord, err := self.doStrideMap(env, lhsValue.AsHTuple(), rhsTuple.AsHTuple())
			if err != nil {
				return nil, err
			}
			strideOffs = append(strideOffs, StrideOffset{
				CoordinateExpression: coord,
				UpperBound:           rhsTuple.Rank(),
			})
		} else {
			if lhsValue.IsRecursive() {
				return nil, fmt.Errorf(
					"structure mismatch at dim %d: coordinate is recursive but shape is a leaf",
					idx)
			}
			strideOffs = append(strideOffs, StrideOffset{
				CoordinateExpression: lhsValue.Leaf(),
				UpperBound:           rhsTuple.Rank(),
			})
		}
	}

	return env.Factory.Stride(strideOffs), nil
}

func (self *strideMap) Run(
	env Env,
	input HTuple,
	tileSize ConstHTuple,
	dtypeSize int,
) (Expr, error) {
	if !common.IsValidDtypeSize(dtypeSize) {
		return nil, fmt.Errorf("invalid dtype size %d, want 1, 2 or 4", dtypeSize)
	}
	out, err := self.doStrideMap(env, input, tileSize)
	if err != nil {
		return nil, err
	}
	return env.Factory.Binary(Mul, out, env.Factory.Const(Value(dtypeSize))), nil
}

// StrideMap maps the layout's coordinate HTuple back to a byte-oriented 1D offset
// expression, scaled by the dtype size.
func StrideMap(env Env, layout *Layout, dtypeSize int) (Expr, error) {
	mapper := strideMap{}
	return mapper.Run(env, layout.Coordinate, layout.Shape, dtypeSize)
}
