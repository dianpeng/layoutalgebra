package f2

// OTuple is a structural total order over expression trees.
//
// The order is lexicographic over a per-node tuple:
//
//	(kind, payload)          for leaves
//	(kind, op, lhs, rhs)     for binary nodes, recursively
//
// kind follows the NodeKind rank (Const < Symbol < Variable < Binary), so the
// payload is the const value for Const, the name for Symbol/Variable, and the
// (op, children) pair recursively for Binary. Structurally distinct
// expressions always compare strictly — no hashing anywhere, so the order is
// total and deterministic across runs.
type OTuple struct {
	kind   int    // NodeKind rank
	number int64  // const value (Const only)
	name   string // symbol/variable name, lexicographic (Symbol/Variable only)
	op     int    // binary operator (Binary only)
	lhs    *OTuple
	rhs    *OTuple
}

func (self OTuple) Less(that OTuple) bool {
	if self.kind != that.kind {
		return self.kind < that.kind
	}
	switch self.kind {
	case int(Const):
		return self.number < that.number
	case int(Symbol), int(Variable):
		return self.name < that.name
	default: // Binary
		if self.op != that.op {
			return self.op < that.op
		}
		if !self.lhs.Equal(*that.lhs) {
			return self.lhs.Less(*that.lhs)
		}
		return self.rhs.Less(*that.rhs)
	}
}

func (self OTuple) Equal(that OTuple) bool {
	if self.kind != that.kind {
		return false
	}
	switch self.kind {
	case int(Const):
		return self.number == that.number
	case int(Symbol), int(Variable):
		return self.name == that.name
	default: // Binary
		return self.op == that.op &&
			self.lhs.Equal(*that.lhs) &&
			self.rhs.Equal(*that.rhs)
	}
}

func (self OTuple) Greater(that OTuple) bool {
	return !self.Less(that) && !self.Equal(that)
}

func (self OTuple) LessEq(that OTuple) bool {
	return self.Less(that) || self.Equal(that)
}

func (self OTuple) GreaterEq(that OTuple) bool {
	return self.Greater(that) || self.Equal(that)
}

func NewOTuple(kind int, number int64) OTuple {
	return OTuple{kind: kind, number: number}
}

func NewNameTuple(kind int, name string) OTuple {
	return OTuple{kind: kind, name: name}
}

func NewBinaryTuple(op int, lhs, rhs OTuple) OTuple {
	l, r := lhs, rhs
	return OTuple{kind: int(Binary), op: op, lhs: &l, rhs: &r}
}
