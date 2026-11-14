package common

const (
	offset64 uint64 = 14695981039346656037
	prime64  uint64 = 1099511628211
)

// ---- internal building blocks ----

func addString(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}

func addUint64(h uint64, v uint64) uint64 {
	for i := 0; i < 8; i++ { // little-endian, byte by byte
		h ^= v & 0xff
		h *= prime64
		v >>= 8
	}
	return h
}

// HashString hashes a single string.
func HashString(s string) int64 {
	return int64(addString(offset64, s))
}

// HashInt64 hashes a single int64.
func HashInt64(v int64) int64 {
	return int64(addUint64(offset64, uint64(v)))
}

// HashInt64Pair hashes two int64 values together.
func HashInt64Pair(a, b int64) int64 {
	h := addUint64(offset64, uint64(a))
	h = addUint64(h, uint64(b))
	return int64(h)
}

// HashStringPair hashes two strings together.
// The length of a is mixed in first so ("ab","c") != ("a","bc").
func HashStringPair(a, b string) int64 {
	h := addUint64(offset64, uint64(len(a)))
	h = addString(h, a)
	h = addString(h, b)
	return int64(h)
}

// Combine merges two existing hash values (boost::hash_combine style).
// Useful when you already have hashes and want to mix them.
func Combine(seed, h int64) int64 {
	s := uint64(seed)
	s ^= uint64(h) + 0x9e3779b97f4a7c15 + (s << 6) + (s >> 2)
	return int64(s)
}
