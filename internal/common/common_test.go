package common

import "testing"

func TestGCD(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{0, 0, 0},
		{0, 5, 5},
		{7, 0, 7},
		{12, 18, 6},
		{18, 12, 6},
		{7, 13, 1},
		{48, 36, 12},
		{1, 1, 1},
		{100, 10, 10},
		{12, -18, 6},  // operands taken by absolute value
		{-12, -18, 6}, // operands taken by absolute value
		{-7, 0, 7},
	}
	for _, c := range cases {
		if got := GCD(c.a, c.b); got != c.want {
			t.Errorf("GCD(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLCM(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{0, 5, 0},
		{4, 6, 12},
		{7, 13, 91},
		{3, 3, 3},
	}
	for _, c := range cases {
		if got := LCM(c.a, c.b); got != c.want {
			t.Errorf("LCM(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestStride(t *testing.T) {
	cases := []struct {
		shape []int
		want  []int
	}{
		{[]int{2, 3, 4}, []int{12, 4, 1}},
		{[]int{5}, []int{1}},
		{[]int{1, 1}, []int{1, 1}},
		{[]int{2, 2, 2, 2}, []int{8, 4, 2, 1}},
	}
	for _, c := range cases {
		got := Stride(c.shape)
		if len(got) != len(c.want) {
			t.Fatalf("Stride(%v) = %v, want %v", c.shape, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("Stride(%v) = %v, want %v", c.shape, got, c.want)
			}
		}
	}

	empty := Stride([]int{})
	if len(empty) != 0 {
		t.Errorf("Stride([]) = %v, want empty", empty)
	}
}

func TestProduct(t *testing.T) {
	if got := Product(2, 3, 4); got != 24 {
		t.Errorf("Product(2,3,4) = %d, want 24", got)
	}
	if got := Product(7); got != 7 {
		t.Errorf("Product(7) = %d, want 7", got)
	}
}

func TestRemoveNil(t *testing.T) {
	a, b, c := 1, 2, 3
	in := []*int{&a, nil, &b, nil, &c}
	out := RemoveNil(in)
	if len(out) != 3 {
		t.Fatalf("RemoveNil kept %d elements, want 3", len(out))
	}
	if out[0] != &a || out[1] != &b || out[2] != &c {
		t.Errorf("RemoveNil returned wrong elements: %v", out)
	}
	if got := RemoveNil[int](nil); len(got) != 0 {
		t.Errorf("RemoveNil(nil) = %v, want empty", got)
	}
}

func TestSliceEqual(t *testing.T) {
	if !SliceEqual([]int{1, 2, 3}, []int{1, 2, 3}) {
		t.Error("SliceEqual equal slices reported unequal")
	}
	if SliceEqual([]int{1, 2}, []int{1, 2, 3}) {
		t.Error("SliceEqual different lengths reported equal")
	}
	if SliceEqual([]int{1, 2, 4}, []int{1, 2, 3}) {
		t.Error("SliceEqual different elements reported equal")
	}
	if !SliceEqual[int](nil, nil) {
		t.Error("SliceEqual(nil, nil) reported unequal")
	}
}

func TestIsValidDtypeSize(t *testing.T) {
	for _, sz := range []int{1, 2, 4} {
		if !IsValidDtypeSize(sz) {
			t.Errorf("IsValidDtypeSize(%d) = false, want true", sz)
		}
	}
	for _, sz := range []int{0, 3, 8, -1} {
		if IsValidDtypeSize(sz) {
			t.Errorf("IsValidDtypeSize(%d) = true, want false", sz)
		}
	}
}
