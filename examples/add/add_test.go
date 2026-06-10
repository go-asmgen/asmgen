package add

import "testing"

func TestAdd(t *testing.T) {
	cases := []struct{ a, b, want int64 }{
		{1, 2, 3},
		{0, 0, 0},
		{-5, 5, 0},
		{1 << 40, 1 << 40, 1 << 41},
	}
	for _, c := range cases {
		if got := add(c.a, c.b); got != c.want {
			t.Errorf("add(%d,%d)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
