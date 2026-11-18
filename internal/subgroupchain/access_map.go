package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
	"github.com/dianpeng/layoutalgebra/internal/common"
)

type accessorMap struct{}

// Do a flatten tuple, ie none htuple mapping with the given bag of variables, notes
// the mapping is opinioned, ie it always tries to map from left to right against the
// dimension from left to right.
func (self *accessorMap) doAccessMapOneDepth(
	env Env,
	bov []Expr,
	tileSize ConstTuple,
	consumeFully bool,
) (HTuple, error) {
	htuple := []Expr{}

	// perform range analysis, this is needed for later work, since we will need to work
	// on parametrized expression, instead of fixed set of nodes
	for _, v := range bov {
		if v == nil {
			continue
		}
		AnalyzeRange(v)
	}

	for _, dimension := range tileSize.Array() {
		dimOfVariable := []StrideOffset{}

		for idx, variable := range bov {
			if variable == nil {
				continue
			}
			rng := variable.Range()
			if rng == nil || rng.IsUnknown() {
				return HTuple{}, fmt.Errorf("invalid range")
			}

			bound := rng.(*RangeInterval).UpperBound()
			divider := common.GCD(bound, dimension) // get gcd
			if divider == 1 {                       // this variable expression is unusable, skip it
				continue
			}

			listOfExpr, err := env.Factory.Factorize(variable, []int{bound / divider, divider})
			if err != nil {
				return HTuple{}, err
			}
			check.Check(len(listOfExpr) == 2)
			check.Check(listOfExpr[1] != nil)
			dimOfVariable = append(
				dimOfVariable,
				StrideOffset{
					CoordinateExpression: listOfExpr[1], // expression generate the coordinate
					UpperBound:           divider,       // divider
				},
			)

			if listOfExpr[0] != nil {
				// we have left quota cannot be used up, saved it for the future usage
				check.Check(bound/divider != 1)
				bov[idx] = listOfExpr[0]
			} else {
				bov[idx] = nil
			}

			dimension = dimension / divider
			if dimension == 1 {
				break
			}
		}

		if dimension != 1 {
			return HTuple{}, fmt.Errorf(
				"dimension extent %d not covered by the bag of variables", dimension)
		}
		htuple = append(htuple, env.Factory.Stride(dimOfVariable))
	}

	if consumeFully {
		leftover := 0
		for _, v := range bov {
			if v != nil {
				leftover++
			}
		}
		if leftover != 0 {
			return HTuple{}, fmt.Errorf(
				"%d variable(s) not fully consumed by the access map", leftover)
		}
	}
	return NewHTuple(htuple), nil
}

func (self *accessorMap) doAccessMap(
	env Env,
	bov []Expr,
	tileSize ConstHTuple,
) (HTuple, error) {
	constTuple, ok := tileSize.Flatten().ToTuple() // get a flatten constTuple
	if !ok {
		return HTuple{}, fmt.Errorf("tile size is not flatten-able")
	}
	// the flat mapping must consume every variable; the per-dimension invariant
	// (dimension fully covered) is enforced inside doAccessMapOneDepth
	root, err := self.doAccessMapOneDepth(env, bov, constTuple, true)
	if err != nil {
		return HTuple{}, err
	}

	// checking weather we need to do recursion if needed, ie certain dimension in input
	// tuple is actuall a recursive htuple. Notes the sub-tile structure is passed as
	// is: doAccessMap flattens internally for the one-depth distribution, and the
	// rebuilt sub-coordinate must mirror the shape's nesting for StrideMap.
	for idx := 0; idx < tileSize.Size(); idx++ {
		component := tileSize.Index(idx)
		if component.IsRecursive() {
			subTileSize := component.AsHTuple()
			v := root.Index(idx)
			variable := v.Leaf()
			sub, err := self.doAccessMap(env, []Expr{variable}, subTileSize)
			if err != nil {
				return HTuple{}, err
			}
			root.SetHTuple(idx, sub)
		}
	}

	return root, nil
}

func (self *accessorMap) Run(
	env Env,
	bov []Coord,
	tileSize ConstHTuple,
) (HTuple, error) {
	// the mapping is a bijection only if the BoV product equals the tile rank;
	// validate up front so an over-sized BoV fails loudly instead of silently
	// dropping high digits
	total := 1
	for _, v := range bov {
		if v.UpperBound() < 2 {
			return HTuple{}, fmt.Errorf(
				"BoV entry (id %d) has trivial upper bound %d and can never be consumed",
				v.(Expr).Id(), v.UpperBound())
		}
		total *= v.UpperBound()
	}
	if rank := tileSize.Rank(); total != rank {
		return HTuple{}, fmt.Errorf(
			"bag of variables product %d does not match tile rank %d",
			total, rank)
	}

	bovExpr := make([]Expr, 0, len(bov))
	for _, v := range bov {
		bovExpr = append(bovExpr, v.(Expr))
	}
	return self.doAccessMap(env, bovExpr, tileSize)
}

// Access maps the bag of variables (BoV) into the coordinate HTuple of the given
// tile size, left to right. Each variable enters the coordinate world as a
// full-dedication Coordinate.
func Access(env Env, bov []*Variable, tileSize ConstHTuple) (HTuple, error) {
	accessor := accessorMap{}
	dedicated := common.Map(bov, func(x *Variable) Coord { return env.Factory.Dedicate(x) })
	return accessor.Run(env, dedicated, tileSize)
}

func AccessWithCoord(env Env, bov []Coord, tileSize ConstHTuple) (HTuple, error) {
	accessor := accessorMap{}
	return accessor.Run(env, bov, tileSize)
}
