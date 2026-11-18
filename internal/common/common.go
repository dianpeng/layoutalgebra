package common

// Integer matches all built-in signed and unsigned integer types,
// plus any named type whose underlying type is one of them.
type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr | ~float32 | ~float64
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

// Product returns the product of all values. It panics on an empty list.
func Product[T Number](value ...T) T {
	out := value[0]
	for _, v := range value[1:] {
		out *= v
	}
	return out
}

// RemoveNil returns a new slice with all nil entries dropped.
func RemoveNil[T any](ptr []*T) []*T {
	out := make([]*T, 0, len(ptr))
	for _, v := range ptr {
		if v != nil {
			out = append(out, v)
		}
	}
	return out
}

// Stride returns the row-major stride vector for the given shape: for shape
// [s0, s1, ..., sn-1] the result is [PI(s1..sn-1), PI(s2..sn-1), ..., 1].
// An empty shape yields an empty stride vector.
func Stride[T Integer](val []T) []T {
	out := make([]T, len(val))
	now := T(1)
	for idx := len(val) - 1; idx >= 0; idx-- {
		out[idx] = now
		now *= val[idx]
	}
	return out
}

// SliceEqual reports whether two slices have the same length and equal
// elements in the same order.
func SliceEqual[T comparable](lhs, rhs []T) bool {
	if len(lhs) != len(rhs) {
		return false
	}
	for idx := range lhs {
		if lhs[idx] != rhs[idx] {
			return false
		}
	}
	return true
}

// Slice returns s[start:end] with Python-style negative indexing: a negative
// index counts from the end (s[-1] is the last element).
func Slice[T any](s []T, start, end int) []T {
	if start < 0 {
		start = len(s) + start
	}
	if end < 0 {
		end = len(s) + end
	}
	return s[start:end]
}

// Min3 returns the minimum of three ordered values.
func Min3[T ~int | ~int64 | ~float64](a, b, c T) T {
	out := a
	if b < out {
		out = b
	}
	if c < out {
		out = c
	}
	return out
}
