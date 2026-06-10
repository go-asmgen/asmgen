package loop

import "testing"

func TestSumI64(t *testing.T) {
	cases := []struct {
		in   []int64
		want int64
	}{
		{nil, 0},
		{[]int64{42}, 42},
		{[]int64{1, 2, 3, 4, 5}, 15},
		{[]int64{-10, 10, -5, 5}, 0},
	}
	for _, c := range cases {
		if got := sumI64(c.in); got != c.want {
			t.Errorf("sumI64(%v)=%d want %d", c.in, got, c.want)
		}
	}
}
