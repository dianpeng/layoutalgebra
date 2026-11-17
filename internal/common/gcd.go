package common

// GCD computes the greatest common divisor of a and b using Euclid's
// algorithm. Operands are taken by absolute value, so the result is always
// non-negative; GCD(0, 0) == 0.
func GCD[T Integer](a, b T) T {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// LCM computes the least common multiple of a and b.
// LCM(x, 0) == LCM(0, x) == 0.
func LCM[T Integer](a, b T) T {
	if a == 0 || b == 0 {
		return 0
	}
	return a / GCD(a, b) * b
}
