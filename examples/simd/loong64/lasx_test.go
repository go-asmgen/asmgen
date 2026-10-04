package simd

import (
	"math"
	"math/rand"
	"testing"
)

// TestLASXFloat holds the hand-encoded fused multiply-adds to math.FMA bit for
// bit in every lane, and the broadcast loads to the element they load.
func TestLASXFloat(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	vals := []float64{0, math.Copysign(0, -1), 1, -1, 0.1, 3, math.MaxFloat64,
		math.SmallestNonzeroFloat64, 1e300, math.Inf(1), math.Inf(-1), math.NaN()}
	for i := 0; i < 4000; i++ {
		vals = append(vals, rng.NormFloat64()*math.Pow(10, float64(rng.Intn(40)-20)))
	}
	pick := func(i int) float64 { return vals[i%len(vals)] }
	same := func(x, y float64) bool {
		return math.Float64bits(x) == math.Float64bits(y) || (math.IsNaN(x) && math.IsNaN(y))
	}
	for i := 0; i < len(vals); i++ {
		var a, b, c, out [4]float64
		for l := range a {
			a[l], b[l], c[l] = pick(i+l), pick(i*7+3+l), pick(i*13+5+l)
		}
		fmaF64x4(&a, &b, &c, &out)
		for l := range out {
			if w := math.FMA(a[l], b[l], c[l]); !same(out[l], w) {
				t.Fatalf("fmaF64x4 lane %d (%v, %v, %v) = %v, want %v", l, a[l], b[l], c[l], out[l], w)
			}
		}
		a2, b2, c2 := [2]float64{a[0], a[1]}, [2]float64{b[0], b[1]}, [2]float64{c[0], c[1]}
		var o2 [2]float64
		fmaF64x2(&a2, &b2, &c2, &o2)
		for l := range o2 {
			if w := math.FMA(a2[l], b2[l], c2[l]); !same(o2[l], w) {
				t.Fatalf("fmaF64x2 lane %d = %v, want %v", l, o2[l], w)
			}
		}
	}
	p := [4]float64{1.5, -2.25, 3.125, -4.0625}
	var o4 [4]float64
	var o2 [2]float64
	bcastF64(&p, &o4, &o2)
	if o4 != [4]float64{p[3], p[3], p[3], p[3]} || o2 != [2]float64{p[1], p[1]} {
		t.Fatalf("broadcast: out4 %v out2 %v", o4, o2)
	}
}
