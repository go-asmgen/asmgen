package fft

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"
)

// The products are Go's own complex multiply, operation for operation and
// separately rounded (GOAMD64=v1 does not fuse), so they are compared bit
// for bit.

func values(n int, seed uint64) []complex128 {
	r := rand.New(rand.NewPCG(seed, 1))
	v := make([]complex128, n)
	for i := range v {
		v[i] = complex(r.NormFloat64(), r.NormFloat64())
	}
	return v
}

func same(a, b complex128) bool {
	return math.Float64bits(real(a)) == math.Float64bits(real(b)) &&
		math.Float64bits(imag(a)) == math.Float64bits(imag(b))
}

func TestCMulSSE2(t *testing.T) {
	for _, n := range []int{1, 2, 7, 64} {
		a, b := values(n, 1), values(n, 2)
		want := make([]complex128, n)
		for i := range a {
			want[i] = a[i] * b[i]
		}
		cmulSSE2(&a[0], &b[0], n)
		for i := range a {
			if !same(a[i], want[i]) {
				t.Fatalf("n=%d, i=%d: %v, Go gives %v", n, i, a[i], want[i])
			}
		}
	}
}

func TestCMulAVX2(t *testing.T) {
	if !hasAVX2() {
		t.Skip("no AVX2")
	}
	a, b := values(32, 3), values(32, 4)
	dst := make([]complex128, 32)
	cmulAVX2(&dst[0], &a[0], &b[0], 8)
	for i := range dst {
		if want := a[i] * b[i]; !same(dst[i], want) {
			t.Fatalf("i=%d: %v, Go gives %v", i, dst[i], want)
		}
	}
}

// sign is the forward transform's rotation sign mask, [0, -0] in each lane
// pair: rotS(z) = -i·z.
var sign = [4]float64{0, math.Copysign(0, -1), 0, math.Copysign(0, -1)}

// TestFFT16 runs a 16-point forward FFT as two radix-4 passes, the first with
// ido = 4, l1 = 1, the final one with l1 = 4, and compares it with the DFT.
func TestFFT16(t *testing.T) {
	if !hasAVX2() {
		t.Skip("no AVX2")
	}
	const n, r, ido = 16, 4, 4
	x := values(n, 5)
	// The pass's twiddles, in one group of four points: twiddle j of point i
	// at (j-1)·4 + i, w_j(i) = exp(-2πi·i·j/16).
	tw := make([]complex128, (r-1)*ido)
	for j := 1; j < r; j++ {
		for i := 0; i < ido; i++ {
			tw[(j-1)*ido+i] = cmplx.Exp(complex(0, -2*math.Pi*float64(i*j)/n))
		}
	}
	mid, y := make([]complex128, n), make([]complex128, n)
	pass4(&x[0], &mid[0], &tw[0], &sign[0], ido, 1)
	last4(&mid[0], &y[0], &sign[0], ido)
	for k := range y {
		var want complex128
		for j := range x {
			want += x[j] * cmplx.Exp(complex(0, -2*math.Pi*float64(j*k)/n))
		}
		if cmplx.Abs(y[k]-want) > 1e-12 {
			t.Errorf("bin %d: %v, the DFT gives %v", k, y[k], want)
		}
	}
}
