package f2

import (
  "fmt"

  "github.com/dianpeng/f2algebra/internal/check"
)

type Matrix interface {
  Size() (int, int)
  Index(int, int) Expr
  Set(int, int, Expr)
}

// This file contains definition for transformation matrix, which are all 0/1 constants
// This is typically used to represent a compile known transformation for certain layouts
type CoeffMat struct {
  v [][]Expr
  m, n int // m by n, (m, n) matrix
}

type ConstMat struct {
  v [][]F2
  m, n int
}

func NewCoeffMatAllZero(m, n int) CoeffMat {
  out := make([][]Expr, 0, m)
  for i := 0; i < m; i++ {
    row := make([]Expr, 0, n)
    for j := 0; j < n; j++ {
      row = append(row, NewConstZero())
    }
    out = append(out, row)
  }
  return CoeffMat{v: out, m: m, n: n}
}

func NewCoeffMat(in [][]Expr) CoeffMat {
  m := len(in)
  check.Check(m > 0)
  n := len(in[0])
  for _, v := range in {
    check.Check(len(v) == n)
  }
  return CoeffMat{v: in, m: m, n: n}
}

func NewConstMat(in [][]F2) ConstMat {
  m := len(in)
  check.Check(m > 0)
  n := len(in[0])
  for _, v := range in {
    check.Check(len(v) == n)
  }
  return ConstMat{v: in, m: m, n: n}
}

func NewCoeffMatFromConstMat(in [][]F2) CoeffMat {
  constMat := NewConstMat(in)
  return constMat.ToCoeffMat()
}

func (self *CoeffMat) On(i, j int) { self.v[i][j] = NewConstOne() }
func (self *CoeffMat) Off(i, j int) { self.v[i][j] = NewConstZero() }

func (self *CoeffMat) Set(i, j  int, e Expr) { self.v[i][j] = e }
func (self *CoeffMat) Index(i, j int) Expr { return self.v[i][j] }
func (self *CoeffMat) Size() (int, int) { return self.m, self.n }

func (self *ConstMat) Size() (int, int) { return self.m, self.n }
func (self *ConstMat) Index(i, j int) Expr { return NewConst(self.v[i][j]) }
func (self *ConstMat) Set(i, j  int, e Expr) {
  check.Check(e.Kind() == Const)
  self.v[i][j] = e.(*NodeConst).Value()
}
func (self *ConstMat) Raw() [][]F2 { return self.v }

func (self *ConstMat) ToCoeffMat() CoeffMat {
  vv := make([][]Expr, 0, self.m)
  for _, row := range self.v {
    out := make([]Expr, 0, len(row))
    for _, val := range row {
      out = append(out, NewConst(val))
    }
    vv = append(vv, out)
  }
  return NewCoeffMat(vv)
}

// Asserting the matrix to be a constant matrix, ie all value are not symbol, but constant
// if so, user can obtain more various interesting property of the matrix, otherwise
// symbol is unknown during abstract interpretation, hence we cannot derive any properties
func (self *CoeffMat) IsConst() bool {
  for _, row := range self.v {
    for _, ele := range row {
      if ele.Kind() == Symbol {
        return false
      }
    }
  }
  return true
}

func (self *CoeffMat) ToConst() (ConstMat, error) {
  vv := make([][]F2, 0, self.m)
  for _, row := range self.v {
    outRow := make([]F2, 0, self.n)
    for _, ele := range row {
      if ele.Kind() == Const {
        outRow = append(outRow, ele.(*NodeConst).Value())
      } else {
        return ConstMat{}, fmt.Errorf("not a constant matrix")
      }
    }
    vv = append(vv, outRow)
  }
  return ConstMat{v: vv, m: self.m, n: self.n}, nil
}

func (self *CoeffMat) constValue(static Expr) F2 {
  check.Check(static.Kind() == Const)
  return static.(*NodeConst).Value()
}

// checking weather the transmat is a permutation matrix. Notes, each row contains just
// one 1, and each column is claimed by exactly one row (WR-03: column uniqueness)
func (self *CoeffMat) IsPerm() bool {
  check.Check(self.IsConst())

  seenCol := make([]bool, self.n)
  for i := 0; i < self.m; i++ {
    numOfOne := 0
    colOfOne := -1
    for j := 0; j < self.n; j++ {
      v := self.constValue(self.v[i][j])
      if v {
        numOfOne++
        colOfOne = j
      }
    }
    if numOfOne != 1 {
      return false
    }
    if seenCol[colOfOne] {
      return false // column already claimed by an earlier row
    }
    seenCol[colOfOne] = true
  }
  return true
}

func (self *CoeffMat) IsIdentity() bool {
  check.Check(self.IsConst())

  // non-square matrices are never the identity; guards self.v[i][i] indexing
  // past the row length when m > n (WR-04)
  if self.m != self.n {
    return false
  }

  for i := 0; i < self.m; i++ {

    // (1) diagnoal element is 1
    if !self.constValue(self.v[i][i]) {
      return false
    }

    // (2) rest are zeros
    for j := 0; j < self.n; j++ {
      if j != i {
        if self.constValue(self.v[i][j]) {
          return false
        }
      }
    }
  }
  return true
}

// --------------------------------------------------------------------------------------
//
// trans mat operations
//
// A{m, n} * X{n * 1}
//
// Add := XOR
// Mul := AND
//
// --------------------------------------------------------------------------------------

func (self *CoeffMat) Apply(that Vector) Vector {
  check.Check(self.n == that.Rank())
  v := []Expr{}

  for i := 0; i < self.m; i++ {
    m := []Expr{}
    // map part, ie element wise multiplication (AND)
    for j := 0; j < self.n; j++ {
      m = append(m, NewBinary(Mul, self.Index(i, j), that.Index(j)))
    }

    // reduce part, ie reduction cross the axis via addition (XOR)
    cur := m[0]
    for _, v := range m[1:] {
      cur = NewBinary(Add, cur, v)
    }

    // now we have reduced result, performs a folding
    v = append(v, cur)
  }
  return NewVectorWithFold(v)
}
