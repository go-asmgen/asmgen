package array

import "testing"

func TestSumI64x3(t *testing.T) {
	if got := sumI64x3([3]int64{10, 20, 30}); got != 60 {
		t.Errorf("sumI64x3 = %d want 60", got)
	}
	if got := sumI64x3([3]int64{-5, 0, 5}); got != 0 {
		t.Errorf("sumI64x3 = %d want 0", got)
	}
}
