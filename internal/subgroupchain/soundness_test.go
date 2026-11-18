package subgroupchain

import "testing"

// --------------------------------------------------------------------------------------
// Soundness stress test
//
// Enumerates every (bag of variables, tile shape) pair with equal product N, for all
// N up to a bound, and asserts:
//
//  1. Completeness: Access succeeds (product equality is sufficient for the greedy
//     GCD-based construction to find a mapping).
//  2. Soundness: the composed Access + StrideMap is a bijection onto [0, N).
//
// This is a property-level test complementing the hand-picked scenario tests.
// --------------------------------------------------------------------------------------

// unorderedFactorizations returns all multisets of integers >= min (and >= 2) whose
// product is n, eg 12 -> [[12] [2 6] [3 4] [2 2 3]].
func unorderedFactorizations(n, min int) [][]int {
	if n == 1 {
		return [][]int{{}}
	}
	var out [][]int
	for f := min; f*f <= n; f++ {
		if n%f != 0 {
			continue
		}
		for _, rest := range unorderedFactorizations(n/f, f) {
			out = append(out, append([]int{f}, rest...))
		}
	}
	out = append(out, []int{n})
	return out
}

// orderedFactorizations returns all ordered tuples of integers >= 2 whose product is
// n, eg 4 -> [[4] [2 2]]; order matters, matching per-dimension tile extents.
func orderedFactorizations(n int) [][]int {
	if n == 1 {
		return [][]int{{}}
	}
	var out [][]int
	out = append(out, []int{n})
	for f := 2; f*f <= n; f++ {
		if n%f != 0 {
			continue
		}
		for _, rest := range orderedFactorizations(n / f) {
			out = append(out, append([]int{f}, rest...))
		}
	}
	return out
}

func TestAccessStrideMapSoundnessExhaustive(t *testing.T) {
	const maxN = 60

	cases := 0
	for n := 2; n <= maxN; n++ {
		bovs := unorderedFactorizations(n, 2)
		tiles := orderedFactorizations(n)

		for _, bov := range bovs {
			for _, tileShape := range tiles {
				cases++
				env := *NewEnv()
				f := env.Factory

				vars := make([]*Variable, 0, len(bov))
				for i, bound := range bov {
					vars = append(vars, f.Variable("bov", varName(i), bound))
				}
				tile := NewConstHTuple(intsToValues(tileShape))

				// 1) completeness: product equality must be sufficient
				coord, err := Access(env, vars, tile)
				if err != nil {
					t.Fatalf("N=%d bov=%v tile=%v: Access rejected: %v",
						n, bov, tileShape, err)
				}

				// IR uniformity: every dimension access expression is a Coord
				// node (a Coordinate or Stride) — never a raw Variable
				for i := 0; i < coord.Size(); i++ {
					dim := coord.Index(i)
					if dim.IsLeaf() && !IsCoord(dim.Leaf()) {
						t.Fatalf("N=%d bov=%v tile=%v: dim %d is not a coordinate access expression",
							n, bov, tileShape, i)
					}
				}

				// 2) soundness: composed map is a bijection onto [0, N)
				offset := mustStrideMap(t, env, &Layout{Coordinate: coord, Shape: tile}, 1)
				seen := make([]bool, n)
				forEachAssignment(bov, func(assignment []int) bool {
					envmap := make(map[string]int, len(bov))
					for i := range bov {
						envmap[varName(i)] = assignment[i]
					}
					got := evalExpr(t, offset, envmap)
					if got < 0 || got >= n {
						t.Fatalf("N=%d bov=%v tile=%v assignment=%v: offset %d out of range",
							n, bov, tileShape, assignment, got)
					}
					if seen[got] {
						t.Fatalf("N=%d bov=%v tile=%v assignment=%v: offset %d collision",
							n, bov, tileShape, assignment, got)
					}
					seen[got] = true
					return true
				})
				for off, s := range seen {
					if !s {
						t.Fatalf("N=%d bov=%v tile=%v: offset %d never produced (not surjective)",
							n, bov, tileShape, off)
					}
				}
			}
		}
	}
	t.Logf("verified %d (bov, tile) pairs with full bijection enumeration", cases)
}

func varName(i int) string {
	// tiny int -> name without importing strconv in test helpers
	if i < 10 {
		return string(rune('a' + i))
	}
	return string(rune('a'+i%26)) + string(rune('0'+i/26))
}

func intsToValues(list []int) []Value {
	out := make([]Value, len(list))
	for i, v := range list {
		out[i] = v
	}
	return out
}

// forEachAssignment enumerates the full product space of the given bounds in mixed
// radix order, invoking fn on each assignment; stops early if fn returns false.
func forEachAssignment(bounds []int, fn func([]int) bool) {
	assignment := make([]int, len(bounds))
	for {
		if !fn(assignment) {
			return
		}
		idx := len(bounds) - 1
		for idx >= 0 {
			assignment[idx]++
			if assignment[idx] < bounds[idx] {
				break
			}
			assignment[idx] = 0
			idx--
		}
		if idx < 0 {
			return
		}
	}
}
