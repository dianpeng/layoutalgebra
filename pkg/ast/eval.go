package ast

import (
	"fmt"
)

// Eval is the reference integer evaluator: it substitutes every Var/Symbol
// from the assignment map and evaluates the tree bottom-up. It is what the
// end consumer effectively does when generating index arithmetic, and what
// tests use to machine-check lowered offsets against the native pipelines.
//
// Error cases (never panics on well-formed trees):
//   - a Var or Symbol with no entry in the assignment
//   - Div/Mod with a zero right operand (detected after the operands
//     evaluate, mirroring the subgroupchain reference interpreter)
//   - Eval of a Tuple: a structural node has no scalar value
func Eval(e Expr, assignment map[string]int) (int, error) {
	switch node := e.(type) {
	case *Const:
		return node.Value, nil

	case *Var:
		v, ok := assignment[node.Name]
		if !ok {
			return 0, fmt.Errorf("variable %q has no assignment", node.Name)
		}
		return v, nil

	case *Symbol:
		v, ok := assignment[node.Name]
		if !ok {
			return 0, fmt.Errorf("symbol %q has no assignment", node.Name)
		}
		return v, nil

	case *Xor:
		l, err := Eval(node.L, assignment)
		if err != nil {
			return 0, err
		}
		r, err := Eval(node.R, assignment)
		if err != nil {
			return 0, err
		}
		return l ^ r, nil

	case *And:
		l, err := Eval(node.L, assignment)
		if err != nil {
			return 0, err
		}
		r, err := Eval(node.R, assignment)
		if err != nil {
			return 0, err
		}
		return l & r, nil

	case *Add:
		l, err := Eval(node.L, assignment)
		if err != nil {
			return 0, err
		}
		r, err := Eval(node.R, assignment)
		if err != nil {
			return 0, err
		}
		return l + r, nil

	case *Sub:
		l, err := Eval(node.L, assignment)
		if err != nil {
			return 0, err
		}
		r, err := Eval(node.R, assignment)
		if err != nil {
			return 0, err
		}
		return l - r, nil

	case *Mul:
		l, err := Eval(node.L, assignment)
		if err != nil {
			return 0, err
		}
		r, err := Eval(node.R, assignment)
		if err != nil {
			return 0, err
		}
		return l * r, nil

	case *Div:
		l, err := Eval(node.L, assignment)
		if err != nil {
			return 0, err
		}
		r, err := Eval(node.R, assignment)
		if err != nil {
			return 0, err
		}
		if r == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return l / r, nil

	case *Mod:
		l, err := Eval(node.L, assignment)
		if err != nil {
			return 0, err
		}
		r, err := Eval(node.R, assignment)
		if err != nil {
			return 0, err
		}
		if r == 0 {
			return 0, fmt.Errorf("modulo by zero")
		}
		return l % r, nil

	case *Tuple:
		return 0, fmt.Errorf("tuple is a structural node and has no scalar value")

	default:
		return 0, fmt.Errorf("unknown ast node %T", e)
	}
}
