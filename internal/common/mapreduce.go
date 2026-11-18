package common

// Map transforms a slice of type T into a slice of type V using fn.
func Map[T any, V any](collection []T, fn func(T) V) []V {
	result := make([]V, len(collection))
	for i, item := range collection {
		result[i] = fn(item)
	}
	return result
}

// Filter returns a slice containing only elements of collection that satisfy predicate.
func Filter[T any](collection []T, predicate func(T) bool) []T {
	result := make([]T, 0, len(collection))
	for _, item := range collection {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

// Reduce reduces a slice of type T into a single value of type V using fn and
// an initial accumulator value.
func Reduce[T any, V any](collection []T, initial V, fn func(acc V, item T) V) V {
	acc := initial
	for _, item := range collection {
		acc = fn(acc, item)
	}
	return acc
}

// Keys returns the keys of m in sorted order, so results derived from map
// iteration stay deterministic.
func Keys[M ~map[K]V, K ~string, V any](m M) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
