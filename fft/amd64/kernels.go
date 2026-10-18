// Package amd64 builds the amd64 FFT kernels of go-fft (github.com/go-fft/fft)
// on top of go-asmgen's amd64 builder: Stockham passes of radix 2, 3, 4, 5, 8,
// 9, 10, 12, 15, 16 and 20 in the interleaved layout (SSE2 stages, AVX2 and
// AVX-512 passes, batched and strided passes), the split layouts at 256 and
// 512 bits, float32 passes, the real-FFT untangle and retangle, pointwise
// complex products, and the stack-frame trampolines that run two or three
// passes in one call.
//
// Each builder appends one TEXT block to an emit.File. What belongs to the
// caller is a parameter: the symbol, the offsets of the constants in the
// tables the kernels read (Consts, SplitConsts), and whether the products are
// fused (Kernels.FMA). What belongs to the kernel is fixed and documented on
// each builder: its argument names and types, which the caller's Go
// declaration must match (go vet's asmdecl checks it), its register use and
// its stack frame.
//
// The arithmetic of every builder is a contract with a scalar reference: the
// operations of the reference, in its order, each separately rounded (or, with
// FMA, fused exactly where the reference calls math.FMA), so that the kernel
// is bit-identical to it. go-fft's tests hold each kernel to its reference;
// this package's tests hold the emitted text to golden files and assemble it.
//
// Constants tables. The interleaved kernels read their constants from a table
// whose rows are 32 bytes (one YMM register; 64 bytes, one ZMM register, for
// the AVX-512 builders, which double every offset): Consts gives the byte
// offset of each row in the 32-byte table. Row Sign is the rotation sign mask,
// which carries the transform direction, so one kernel serves both
// directions. The split-layout kernels read a table of their own
// (SplitConsts), which does not depend on the direction.
package amd64

import (
	"fmt"
	"slices"
)

// Consts gives the byte offsets of the rows of the interleaved kernels'
// constants table, for 32-byte rows. Each row holds one value in every lane,
// except the sign masks and the ss rows, whose lanes alternate.
type Consts struct {
	// Sign is the rotation sign mask: [0, -0] repeated for the forward
	// transform (rotS(z) = -i·z), [-0, 0] for the inverse. The real-FFT
	// untangle kernels read the forward table: it is their imaginary-lane
	// sign flip.
	Sign int
	// Half is 0.5, Sin120 sin(2π/3), C51, C52, S51 and S52 cos(2π/5),
	// cos(4π/5), sin(2π/5) and sin(4π/5), H √2/2.
	Half, Sin120, C51, C52, S51, S52, H int
	// AddSub, read by the AVX-512 builders only, is [-0, 0] repeated: the
	// real-lane sign flip that stands in for VADDSUBPD, which has no 512-bit
	// form.
	AddSub int
	// The radix-16 butterfly's internal twiddles W16^e = cos θ + s·i·sin θ,
	// θ = 2π·e/16, s the direction sign: the cosines R16C1 = cos(π/8) (e =
	// 1), R16S1 = sin(π/8) (e = 3), R16NH = −√2/2 (e = 6) and R16NC1 =
	// −cos(π/8) (e = 9) in every lane (H is e = 2's), and the rows [−ss, ss]
	// with ss = s·sin θ: R16SS1 (e = 1), R16SSH (e = 2 and 6), R16SSC1 (e =
	// 3) and R16SSN1 (e = 9).
	R16C1, R16S1, R16NH, R16NC1, R16SS1, R16SSH, R16SSC1, R16SSN1 int
	// The radix-9 butterfly's internal twiddles W9^e for e = 1, 2, 4: the
	// cosine R9C<e> in every lane and the row R9S<e> = [−ss, ss], ss =
	// s·sin(2π·e/9).
	R9C1, R9S1, R9C2, R9S2, R9C4, R9S4 int
	// R32 is the first of the radix-32 final pass's rows (LastR32, go-fft
	// Round 42): for each exponent e of r32Es in turn, a row cos θ in
	// every lane, then a row [−ss, ss], ss = s·sin θ, θ = 2π·e/32 (two
	// rows per exponent, 28 in all).
	R32 int
}

// SplitConsts gives the byte offsets of the 32-byte rows of the split-layout
// kernels' constants table (one value in every lane): H √2/2, Neg −0 (the
// sign flip), and the radix-3 and radix-5 constants Half 0.5, Sin120
// sin(2π/3), C51, C52, S51 and S52 cos(2π/5), cos(4π/5), sin(2π/5) and
// sin(4π/5). The 512-bit split kernels broadcast H and Neg from the same rows.
// R32 is the first of seven rows C_k = cos(kπ/16), k = 1 .. 7, which the
// radix-32 kernels read (SplitR32, go-fft Round 42).
type SplitConsts struct {
	H, Neg, Half, Sin120, C51, C52, S51, S52, R32 int
}

// Kernels builds the kernels that read a constants table.
type Kernels struct {
	// K is the layout of the interleaved kernels' table.
	K Consts
	// SplitK is the layout of the split-layout kernels' table.
	SplitK SplitConsts
	// FMA builds the fused twins of the float64 pass kernels: the twiddle
	// product, re = fma(yr, wr, −(yi·wi)) and im = fma(yi, wr, yr·wi), and
	// the constant products of radix 8 (a ± h·s as fma(±h, s, a)) and of the
	// radix-9 and -16 internal twiddles (fma(c, x, swap(x)·[−ss, ss])), each
	// with one rounding. Everything else is the default kernels' own, so a
	// final pass of radix 2, 3, 4, 5, 10, 12, 15 or 20, which has none of
	// those products, is the same code either way. The float32, untangle and
	// SSE2 builders do not read it. The kernels it builds use FMA3 and must
	// be gated on it (asm.FeatureProbe with asm.FMA).
	FMA bool
}

// check panics unless r is one of the radices a builder supports.
func check(builder string, r int, radices ...int) {
	if !slices.Contains(radices, r) {
		panic(fmt.Sprintf("fft/amd64: %s: radix %d not in %v", builder, r, radices))
	}
}

// fp addresses argument name of a kernel whose arguments, names, are all
// eight bytes wide: name+8·i(FP).
func fp(names []string, name string) string {
	i := slices.Index(names, name)
	if i < 0 {
		panic(fmt.Sprintf("fft/amd64: no argument %s in %v", name, names))
	}
	return fmt.Sprintf("%s+%d(FP)", name, 8*i)
}
