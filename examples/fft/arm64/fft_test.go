package fft

import (
	"math"
	"math/cmplx"
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

// TestCMulNEON compares the kernel with Go's complex multiply bit for bit:
// gc fuses it on arm64 (FMSUBD, FMADDD) and the kernel fuses the same
// products.
func TestCMulNEON(t *testing.T) {
	for _, n := range []int{1, 2, 7, 64} {
		a, b := values(n, 1), values(n, 2)
		want := make([]complex128, n)
		for i := range a {
			want[i] = a[i] * b[i]
		}
		cmulNEON(&a[0], &b[0], n)
		for i := range a {
			if math.Float64bits(real(a[i])) != math.Float64bits(real(want[i])) ||
				math.Float64bits(imag(a[i])) != math.Float64bits(imag(want[i])) {
				t.Fatalf("n=%d, i=%d: %v, Go gives %v", n, i, a[i], want[i])
			}
		}
	}
}

// TestFFT16 runs a 16-point forward FFT as two radix-4 passes, the first with
// ido = 4, l1 = 1, the final one with l1 = 4, and compares it with the DFT.
func TestFFT16(t *testing.T) {
	const n, r, ido = 16, 4, 4
	x := values(n, 5)
	// The twiddles w_j(i) = exp(-2πi·i·j/16), for each pair of points i,
	// i+1 and each j: re(w_j(i)), re(w_j(i+1)), im(w_j(i)), im(w_j(i+1)).
	var tw []float64
	for i := 0; i < ido; i += 2 {
		for j := 1; j < r; j++ {
			w0 := cmplx.Exp(complex(0, -2*math.Pi*float64(i*j)/n))
			w1 := cmplx.Exp(complex(0, -2*math.Pi*float64((i+1)*j)/n))
			tw = append(tw, real(w0), real(w1), imag(w0), imag(w1))
		}
	}
	mid, y := make([]complex128, n), make([]complex128, n)
	pass4(&x[0], &mid[0], &tw[0], ido, 1)
	last4(&mid[0], &y[0], ido)
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
