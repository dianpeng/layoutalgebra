package subgroupchain

import "fmt"

// Eval is the reference interpreter for the subgroupchain expression IR: it
// concretely assigns every external variable and evaluates the expression
// bottom-up. It is what the end consumer effectively does when generating
// code from a mapped offset expression, and it backs the framework's sweep
// predicates (sector counts, bank distinctness).
//
// Missing variable assignments, division/modulo by zero are errors.
func Eval(e Expr, assignment map[string]int) (int, error) {
	switch n := e.(type) {
	case *Const:
		return n.Value(), nil

	case *Variable:
		v, ok := assignment[n.Name()]
		if !ok {
			return 0, fmt.Errorf("variable %q has no assignment", n.Name())
		}
		return v, nil

	case *Symbol:
		v, ok := assignment[n.Name()]
		if !ok {
			return 0, fmt.Errorf("symbol %q has no assignment", n.Name())
		}
		return v, nil

	case *Binary:
		l, err := Eval(n.L(), assignment)
		if err != nil {
			return 0, err
		}
		r, err := Eval(n.R(), assignment)
		if err != nil {
			return 0, err
		}
		switch n.Op() {
		case Add:
			return l + r, nil
		case Sub:
			return l - r, nil
		case Mul:
			return l * r, nil
		case Div:
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return l / r, nil
		case Mod:
			if r == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			return l % r, nil
		default:
			return 0, fmt.Errorf("unknown binary op %d", n.Op())
		}

	case *Coordinate:
		// radix based digit extraction: (source % radix) / stride
		src, err := Eval(n.Source(), assignment)
		if err != nil {
			return 0, err
		}
		return (src % int(n.Radix())) / int(n.Stride()), nil

	case *Stride:
		total := 0
		stride := n.Stride()
		for i, c := range n.Coord() {
			v, err := Eval(c, assignment)
			if err != nil {
				return 0, err
			}
			total += v * stride[i]
		}
		return total, nil

	default:
		return 0, fmt.Errorf("unknown expression node kind %d", e.Kind())
	}
}
