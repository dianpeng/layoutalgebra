package common

// Integer matches all built-in signed and unsigned integer types,
// plus any named type whose underlying type is one of them.
type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// IsPowerOf2 reports whether n is a power of two (1, 2, 4, 8, ...).
// Zero and negative numbers are never powers of two.
func IsPowerOf2[T Integer](n T) bool {
	return n > 0 && n&(n-1) == 0
}

func Repeat[T any](N int, val T) []T {
  out := make([]T, 0, N)
  for i := 0; i < N; i++ {
    out = append(out, val)
  }
  return out
}

func Select[T any](cond bool, lhs, rhs T) T {
  if cond {
    return lhs
  } else {
    return rhs
  }
}
