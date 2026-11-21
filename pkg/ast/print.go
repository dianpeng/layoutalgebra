package ast

import (
	"fmt"
	"strconv"
	"strings"
)

// String methods — infix, fully parenthesized so distinct shapes always
// render distinctly. A Var with a family renders as "family.name"; a bare
// Var renders as its name alone.

func (self *Const) String() string { return strconv.Itoa(self.Value) }

func (self *Symbol) String() string { return self.Name }

func (self *Var) String() string {
	if self.Family == "" {
		return self.Name
	}
	return self.Family + "." + self.Name
}

func (self *Xor) String() string {
	return "(" + self.L.String() + " ^ " + self.R.String() + ")"
}

func (self *And) String() string {
	return "(" + self.L.String() + " & " + self.R.String() + ")"
}

func (self *Add) String() string {
	return "(" + self.L.String() + " + " + self.R.String() + ")"
}

func (self *Sub) String() string {
	return "(" + self.L.String() + " - " + self.R.String() + ")"
}

func (self *Mul) String() string {
	return "(" + self.L.String() + " * " + self.R.String() + ")"
}

func (self *Div) String() string {
	return "(" + self.L.String() + " / " + self.R.String() + ")"
}

func (self *Mod) String() string {
	return "(" + self.L.String() + " % " + self.R.String() + ")"
}

func (self *Tuple) String() string {
	parts := make([]string, 0, len(self.Elem))
	for _, e := range self.Elem {
		parts = append(parts, e.String())
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// Dump renders the tree in a multi-line prefix form — one line per node,
// children indented two spaces deeper than their parent. Meant for human
// inspection of deep trees where the infix String form becomes unreadable.
func Dump(e Expr) string {
	var b strings.Builder
	dumpInto(&b, e, 0)
	return b.String()
}

func dumpInto(b *strings.Builder, e Expr, depth int) {
	b.WriteString(strings.Repeat("  ", depth))
	switch node := e.(type) {
	case nil:
		b.WriteString("<nil>")
	case *Const:
		fmt.Fprintf(b, "const %d", node.Value)
	case *Symbol:
		fmt.Fprintf(b, "symbol %s", node.Name)
	case *Var:
		if node.Family == "" {
			fmt.Fprintf(b, "var %s", node.Name)
		} else {
			fmt.Fprintf(b, "var %s.%s", node.Family, node.Name)
		}
	case *Xor:
		b.WriteString("xor")
	case *And:
		b.WriteString("and")
	case *Add:
		b.WriteString("add")
	case *Sub:
		b.WriteString("sub")
	case *Mul:
		b.WriteString("mul")
	case *Div:
		b.WriteString("div")
	case *Mod:
		b.WriteString("mod")
	case *Tuple:
		fmt.Fprintf(b, "tuple %d", len(node.Elem))
	default:
		fmt.Fprintf(b, "unknown %T", e)
	}
	b.WriteString("\n")
	for _, child := range Children(e) {
		dumpInto(b, child, depth+1)
	}
}
