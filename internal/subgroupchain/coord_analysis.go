package subgroupchain

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra/internal/check"
	"github.com/dianpeng/layoutalgebra/internal/common"
)

type coordAnalysis struct{}

func (self *coordAnalysis) run(root Expr) {
	VisitPost(
		root,
		func(v Expr) bool { return IsCoord(v) },
		func(v Expr) error {
			coord := v.(Coord)
			info := coord.CoordInfo()
			info.Reset()

			children := coord.CoordChildren()

			if len(children) == 0 {
				// chain terminal: a Coordinate sourcing a raw Variable.
				// Notes a Stride always has operands, so only a terminal
				// Coordinate lands here.
				c, ok := v.(*Coordinate)
				check.Check(ok, "chain terminal must be a Coordinate node")
				src, ok := c.Source().(*Variable)
				check.Check(ok, "terminal Coordinate must source a Variable")
				info.Family = []string{src.Family()}
				info.Name = []string{src.Name()}
				// Partial: false only for a full dedication of the variable,
				// ie the coordinate exhausts the variable's whole range
				info.Partial = c.UpperBound() != src.UpperBound()
			} else {
				// composite: merge the children's infos, which the post-order
				// walk has already populated
				family := make(map[string]bool)
				name := make(map[string]bool)
				for _, ch := range children {
					for _, f := range ch.CoordInfo().Name {
						name[f] = true
					}
					for _, f := range ch.CoordInfo().Family {
						family[f] = true
					}
				}
				info.Partial = true
				info.Family = common.Keys(family)
				info.Name = common.Keys(name)
			}
			info.UpperBound = int(coord.UpperBound())
			return nil
		},
	)
}

// This function will perform coordinate analysis, and expect the root expression to be
// a Coord. It will return the root Coord along with unexpected error
func AnalyzeCoord(x Expr) (Coord, error) {
	if !IsCoord(x) {
		return nil, fmt.Errorf("root must be a valid Coord node")
	}
	ca := coordAnalysis{}
	ca.run(x)
	return x.(Coord), nil
}
