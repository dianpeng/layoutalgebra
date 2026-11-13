package f2

import (
  "strconv"
  "strings"
)

// --------------------------------------------------------------------------------------
//
// Human readable, diff friendly dump facilities for the F2 algebra.
//
// (1) String() is part of the Expr interface (and fmt.Stringer on F2): infix
//     render, fully parenthesized at every binary node, so any tree reads
//     unambiguously and any Expr-typed value prints via fmt directly.
// (2) DumpPrefix: operator-first fully parenthesized prefix render of any Expr.
// (3) Dump() on Vector/CoeffMat/ConstMat: one line per bit / per matrix row, so
//     successive pipeline stages (hw bits -> MapAccess -> StrideMap -> CoeffMatAll
//     -> elimination -> SwizzleMap) can be diffed as exact multi-line text.
//
// All rendering is strictly read-only traversal: never touches SetMarker/Marker,
// Commute, Fold or Replace, hence it is safe on any tree at any pipeline stage.
//
// --------------------------------------------------------------------------------------

// dumpOpSym maps an operator constant to its infix/prefix symbol. The algebra
// defines exactly two operators (Add = Xor, Mul = And), so no other case exists.
func dumpOpSym(op int) string {
  if op == Xor {
    return "^"
  }
  return "*"
}

// String renders the scalar as "0" or "1".
func (self F2) String() string {
  if self.IsOne() {
    return "1"
  }
  return "0"
}

// String renders the variable's name verbatim, eg "l_0".
func (self *NodeVariable) String() string { return self.Name() }

// String renders the symbol's name verbatim, eg "M".
func (self *NodeSymbol) String() string { return self.Name() }

// String renders the constant as "0" or "1".
func (self *NodeConst) String() string {
  if self.Value().IsOne() {
    return "1"
  }
  return "0"
}

// String renders a binary node fully parenthesized in infix form, eg
// Xor(a, Mul(b, c)) renders "(a ^ (b * c))".
func (self *NodeBinary) String() string {
  return "(" + self.L().String() + " " + dumpOpSym(self.Op()) + " " + self.R().String() + ")"
}

// DumpPrefix renders any expression tree in operator-first fully parenthesized
// prefix form, eg Xor(a, Mul(b, c)) renders "(^ a (* b c))". Leaves render
// identically to String().
func DumpPrefix(e Expr) string {
  if bin, ok := e.(*NodeBinary); ok {
    return "(" + dumpOpSym(bin.Op()) + " " + DumpPrefix(bin.L()) + " " + DumpPrefix(bin.R()) + ")"
  }
  return e.String()
}

// Dump renders the vector as one "bit i: <expr>" line per bit index, infix
// render, trailing newline on the last line, no header.
func (self *Vector) Dump() string {
  var b strings.Builder
  for i := 0; i < self.Size(); i++ {
    b.WriteString("bit ")
    b.WriteString(strconv.Itoa(i))
    b.WriteString(": ")
    b.WriteString(self.Index(i).String())
    b.WriteString("\n")
  }
  return b.String()
}

// Dump renders the coefficient matrix as m rows of bracketed, space-separated
// infix renders, one row per line, trailing newline on the last row.
func (self *CoeffMat) Dump() string {
  m, n := self.Size()
  var b strings.Builder
  for i := 0; i < m; i++ {
    b.WriteString("[")
    for j := 0; j < n; j++ {
      if j > 0 {
        b.WriteString(" ")
      }
      b.WriteString(self.Index(i, j).String())
    }
    b.WriteString("]\n")
  }
  return b.String()
}

// Dump renders the constant matrix as m rows of bracketed, space-separated 0/1
// digits, one row per line, trailing newline on the last row.
func (self *ConstMat) Dump() string {
  m, n := self.Size()
  raw := self.Raw()
  var b strings.Builder
  for i := 0; i < m; i++ {
    b.WriteString("[")
    for j := 0; j < n; j++ {
      if j > 0 {
        b.WriteString(" ")
      }
      b.WriteString(raw[i][j].String())
    }
    b.WriteString("]\n")
  }
  return b.String()
}
