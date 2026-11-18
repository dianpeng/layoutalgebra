package subgroupchain

// Facilities to walk expression trees. Notes the expr is always a tree, never a
// DAG or cyclical graph.

const (
	OrderPost = iota
	OrderPre
)

type orderPost struct {
	out []Expr
}

type orderPre struct {
	out []Expr
}

func (self *orderPost) visit(current Expr) {
	switch node := current.(type) {
	case *Binary:
		self.visit(node.L())
		self.visit(node.R())
	case *Stride:
		for _, c := range node.Coord() {
			self.visit(c)
		}
	case *Coordinate:
		self.visit(node.Value())
	}
	self.out = append(self.out, current)
}

func (self *orderPost) Run(current Expr) []Expr {
	self.out = make([]Expr, 0, 32)
	self.visit(current)
	return self.out
}

func orderExprPost(root Expr) []Expr {
	x := orderPost{}
	return x.Run(root)
}

func (self *orderPre) visit(current Expr) {
	self.out = append(self.out, current)
	switch node := current.(type) {
	case *Binary:
		self.visit(node.L())
		self.visit(node.R())
	case *Stride:
		for _, c := range node.Coord() {
			self.visit(c)
		}
	case *Coordinate:
		self.visit(node.Value())
	}
}

func (self *orderPre) Run(current Expr) []Expr {
	self.out = make([]Expr, 0, 32)
	self.visit(current)
	return self.out
}

func orderExprPre(root Expr) []Expr {
	x := orderPre{}
	return x.Run(root)
}

func orderExpr(root Expr, order int) []Expr {
	switch order {
	case OrderPost:
		return orderExprPost(root)
	case OrderPre:
		return orderExprPre(root)
	default:
		return nil
	}
}

func VisitBy(order int, root Expr, predicate func(Expr) bool, visitor func(Expr) error) error {
	orderList := orderExpr(root, order)
	for _, expr := range orderList {
		if predicate(expr) {
			if err := visitor(expr); err != nil {
				return err
			}
		}
	}
	return nil
}

func Visit(
	root Expr,
	predicate func(Expr) bool,
	visitor func(Expr) error,
) error {
	return VisitBy(OrderPre, root, predicate, visitor)
}

func VisitPost(
	root Expr,
	predicate func(Expr) bool,
	visitor func(Expr) error,
) error {
	return VisitBy(OrderPost, root, predicate, visitor)
}
