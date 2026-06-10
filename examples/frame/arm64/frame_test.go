package frame

import "testing"

func TestSpillSum(t *testing.T) {
	if got := spillSum(3, 4); got != 7 {
		t.Errorf("spillSum(3,4)=%d want 7", got)
	}
	if got := spillSum(1<<40, -(1 << 40)); got != 0 {
		t.Errorf("spillSum(2^40,-2^40)=%d want 0", got)
	}
}
