package fft

import (
	"math"
	"math/rand/v2"
	"testing"
)

func values(n int, seed uint64) []complex128 {
	r := rand.New(rand.NewPCG(seed, 1))
	v := make([]complex128, n)
	for i := range v {
		v[i] = complex(r.NormFloat64(), r.NormFloat64())
	}
	return v
}

// TestCMulRVV compares the kernel with Go's complex multiply bit for bit: gc
// fuses it on riscv64 and the kernel fuses the same products.
func TestCMulRVV(t *testing.T) {
	for _, n := range []int{1, 2, 7, 64} {
		a, b := values(n, 1), values(n, 2)
		want := make([]complex128, n)
		for i := range a {
			want[i] = a[i] * b[i]
		}
		cmulRVV(&a[0], &b[0], n)
		for i := range a {
			if math.Float64bits(real(a[i])) != math.Float64bits(real(want[i])) ||
				math.Float64bits(imag(a[i])) != math.Float64bits(imag(want[i])) {
				t.Fatalf("n=%d, i=%d: %v, Go gives %v", n, i, a[i], want[i])
			}
		}
	}
}
