package simd

import (
	"math"
	"math/rand"
	"testing"
)

// TestVSXFloatKernels runs the hand-encoded VSX float64 instructions and holds
// every lane to Go's own arithmetic bit for bit: + - × ÷ and sqrt are single
// correctly rounded IEEE operations, and xvmaddadp to math.FMA (one rounding).
// max/min are checked on numbers only (their NaN rule is the ISA's, not Go's),
// including the signed zeros.
func TestVSXFloatKernels(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	vals := []float64{0, math.Copysign(0, -1), 1, -1, 0.1, 3, math.MaxFloat64,
		math.SmallestNonzeroFloat64, -2.5e-310, 1e300, math.Inf(1), math.Inf(-1)}
	for i := 0; i < 2000; i++ {
		vals = append(vals, rng.NormFloat64()*math.Pow(10, float64(rng.Intn(40)-20)))
	}
	same := func(x, y float64) bool {
		return math.Float64bits(x) == math.Float64bits(y) || (math.IsNaN(x) && math.IsNaN(y))
	}
	for i := 0; i+3 < len(vals); i += 2 {
		a := [2]float64{vals[i], vals[i+1]}
		b := [2]float64{vals[(i*7+3)%len(vals)], vals[(i*13+5)%len(vals)]}
		c := [2]float64{vals[(i*5+1)%len(vals)], vals[(i*11+2)%len(vals)]}
		var out [2]float64
		check := func(name string, k func(), want func(x, y, z float64) float64, numbersOnly bool) {
			k()
			for l := 0; l < 2; l++ {
				if numbersOnly && (math.IsNaN(a[l]) || math.IsNaN(b[l])) {
					continue
				}
				if w := want(a[l], b[l], c[l]); !same(out[l], w) {
					t.Fatalf("%s lane %d (%v, %v, %v) = %v, want %v", name, l, a[l], b[l], c[l], out[l], w)
				}
			}
		}
		check("vadd2", func() { vadd2(&a, &b, &out) }, func(x, y, _ float64) float64 { return x + y }, false)
		check("vsub2", func() { vsub2(&a, &b, &out) }, func(x, y, _ float64) float64 { return x - y }, false)
		check("vmul2", func() { vmul2(&a, &b, &out) }, func(x, y, _ float64) float64 { return float64(x * y) }, false)
		check("vdiv2", func() { vdiv2(&a, &b, &out) }, func(x, y, _ float64) float64 { return x / y }, false)
		check("vsqrt2", func() { vsqrt2(&a, &b, &out) }, func(_, y, _ float64) float64 { return math.Sqrt(y) }, false)
		check("vfma2", func() { vfma2(&a, &b, &c, &out) }, func(x, y, z float64) float64 { return math.FMA(x, y, z) }, false)
		check("vmax2", func() { vmax2(&a, &b, &out) }, func(x, y, _ float64) float64 { return max(x, y) }, true)
		check("vmin2", func() { vmin2(&a, &b, &out) }, func(x, y, _ float64) float64 { return min(x, y) }, true)
	}
}
