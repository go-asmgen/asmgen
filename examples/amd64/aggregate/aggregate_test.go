package aggregate

import "testing"

func TestPairSum(t *testing.T) {
	if got := pairSum(Pair{A: 3, B: 4}); got != 7 {
		t.Errorf("pairSum({3,4})=%d want 7", got)
	}
	if got := pairSum(Pair{A: -10, B: 10}); got != 0 {
		t.Errorf("pairSum({-10,10})=%d want 0", got)
	}
}

func TestMixedN(t *testing.T) {
	if got := mixedN(Mixed{Flag: 1, N: 42}); got != 42 {
		t.Errorf("mixedN({1,42})=%d want 42", got)
	}
	if got := mixedN(Mixed{Flag: -1, N: -9}); got != -9 {
		t.Errorf("mixedN({-1,-9})=%d want -9", got)
	}
}

func TestSliceLen(t *testing.T) {
	if got := sliceLen([]int64{10, 20, 30}); got != 3 {
		t.Errorf("sliceLen(len 3)=%d want 3", got)
	}
	if got := sliceLen([]int64{}); got != 0 {
		t.Errorf("sliceLen(empty)=%d want 0", got)
	}
}

func TestSliceFirst(t *testing.T) {
	if got := sliceFirst([]int64{99, 2, 3}); got != 99 {
		t.Errorf("sliceFirst=%d want 99", got)
	}
	if got := sliceFirst([]int64{-7}); got != -7 {
		t.Errorf("sliceFirst(single)=%d want -7", got)
	}
}

func TestStrLen(t *testing.T) {
	if got := strLen("hello"); got != 5 {
		t.Errorf("strLen(hello)=%d want 5", got)
	}
	if got := strLen(""); got != 0 {
		t.Errorf("strLen(empty)=%d want 0", got)
	}
}
