package subgroupchain

import (
	"fmt"

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

			if variableNode, ok := v.(*Variable); ok {
				info.Family = []string{variableNode.Family()}
				info.Name = []string{variableNode.Name()}
				info.Partial = false
			} else {
				family := make(map[string]bool)
				name := make(map[string]bool)

				// (1) collect from direct coord children, whose info has been
				// populated already (post-order)
				for _, c := range coord.CoordChildren() {
					for _, f := range c.CoordInfo().Name {
						name[f] = true
					}
					for _, f := range c.CoordInfo().Family {
						family[f] = true
					}
				}

				// (2) a Coordinate may wrap a plain factorization expression
				// (Binary), which is not a Coord node: scan its subtree for
				// the underlying Variables instead
				if coordNode, ok := v.(*Coordinate); ok {
					Visit(
						coordNode.Value(),
						func(e Expr) bool { return true },
						func(e Expr) error {
							if varNode, ok := e.(*Variable); ok {
								name[varNode.Name()] = true
								family[varNode.Family()] = true
							}
							return nil
						},
					)
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
